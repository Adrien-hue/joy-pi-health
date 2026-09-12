BEGIN {
    FS = "\t"
    OFS = "\t"
    numeric = "^[-+]?[0-9]+([.][0-9]+)?$"
    unsigned_integer = "^[0-9]+$"
    signed_integer = "^-?[0-9]+$"
    tolerance = 0.000002
}

function measurement_invalid(message, affected_phase) {
    invalid = 1
    if (affected_phase != "")
        phase_invalid[affected_phase] = 1
    if (invalid_reason == "")
        invalid_reason = message
}

function absolute(value) {
    return value < 0 ? -value : value
}

function number_ok(value) {
    return value ~ numeric
}

NR == 1 {
    expected = "phase\tsample\tmonotonic_start_seconds\tmonotonic_end_seconds\tinterval_seconds\tbusy_delta\ttotal_delta\tslice_reference_percent\tservice_percent\tsnapshot_timestamp"
    if ($0 != expected)
        measurement_invalid("invalid cpu-samples.tsv header")
    next
}

{
    phase = $1
    sample = $2
    key = phase SUBSEP sample

    if (NF != 10) {
        measurement_invalid("sample row does not have 10 fields", phase)
        next
    }
    if (phase != "idle" && phase != "half" && phase != "full") {
        measurement_invalid("unknown phase")
        next
    }
    if (sample !~ unsigned_integer || sample + 0 != count[phase] + 1) {
        measurement_invalid("invalid or non-consecutive sample number", phase)
        next
    }
    if (!number_ok($3) || !number_ok($4) || !number_ok($5) ||
        $6 !~ signed_integer || $7 !~ signed_integer || !number_ok($8) ||
        !number_ok($9) || $10 == "") {
        measurement_invalid("malformed sample value", phase)
        next
    }

    start = $3 + 0
    stop = $4 + 0
    interval = $5 + 0
    busy = $6 + 0
    total = $7 + 0
    reference = $8 + 0
    service = $9 + 0

    if (start < 0 || stop <= start || absolute((stop - start) - interval) > tolerance)
        measurement_invalid("invalid monotonic interval", phase)
    if (interval < 0.9 || interval > 1.1)
        measurement_invalid("sample interval is outside 0.9-1.1 seconds", phase)
    if (busy < 0 || total <= 0 || busy > total)
        measurement_invalid("invalid CPU counter delta", phase)
    calculated = total > 0 ? 100 * busy / total : -1
    if (calculated < 0 || absolute(calculated - reference) > tolerance)
        measurement_invalid("slice reference is inconsistent with counter deltas", phase)
    if (reference < 0 || reference > 100 || service < 0 || service > 100)
        measurement_invalid("CPU percentage is outside 0-100", phase)

    count[phase]++
    busy_sum[phase] += busy
    total_sum[phase] += total
    ref[key] = reference
    svc[key] = service
}

END {
    if (NR == 0)
        measurement_invalid("empty cpu-samples.tsv")
    if (count["idle"] != 10 || count["half"] != 10 || count["full"] != 5) {
        measurement_invalid("expected 10 idle, 10 half, and 5 full samples")
        if (count["idle"] != 10) phase_invalid["idle"] = 1
        if (count["half"] != 10) phase_invalid["half"] = 1
        if (count["full"] != 5) phase_invalid["full"] = 1
    }

    phases[1] = "idle"
    phases[2] = "half"
    phases[3] = "full"
    for (p = 1; p <= 3; p++) {
        phase = phases[p]
        aggregate[phase] = total_sum[phase] > 0 ? 100 * busy_sum[phase] / total_sum[phase] : -1
        minimum[phase] = 101
        maximum[phase] = -1
        max_deviation[phase] = 0
        for (i = 1; i <= count[phase]; i++) {
            key = phase SUBSEP i
            if (ref[key] < minimum[phase]) minimum[phase] = ref[key]
            if (ref[key] > maximum[phase]) maximum[phase] = ref[key]
            deviation = absolute(ref[key] - aggregate[phase])
            if (deviation > max_deviation[phase]) max_deviation[phase] = deviation
        }
    }

    if (aggregate["idle"] > 10)
        measurement_invalid("idle aggregate exceeds 10 percent", "idle")
    if (aggregate["half"] < 45 || aggregate["half"] > 55)
        measurement_invalid("half-load aggregate is outside 45-55 percent", "half")
    if (max_deviation["idle"] > 5)
        measurement_invalid("idle slices are not stable within 5 percentage points", "idle")
    if (max_deviation["half"] > 5)
        measurement_invalid("half-load slices are not stable within 5 percentage points", "half")
    if (minimum["full"] < 90 || aggregate["full"] < 90)
        measurement_invalid("full-load reference did not reach 90 percent", "full")

    print "phase", "samples", "aggregate_reference_percent", "minimum_slice_percent", "maximum_slice_percent", "maximum_deviation_percent", "workload_valid" > plateaus
    for (p = 1; p <= 3; p++) {
        phase = phases[p]
        phase_valid = "yes"
        if (phase_invalid[phase] || (phase == "idle" && (aggregate[phase] > 10 || max_deviation[phase] > 5)) ||
            (phase == "half" && (aggregate[phase] < 45 || aggregate[phase] > 55 || max_deviation[phase] > 5)) ||
            (phase == "full" && (minimum[phase] < 90 || aggregate[phase] < 90)))
            phase_valid = "no"
        printf "%s\t%d\t%.6f\t%.6f\t%.6f\t%.6f\t%s\n", phase, count[phase], aggregate[phase], minimum[phase], maximum[phase], max_deviation[phase], phase_valid > plateaus
    }

    print "phase", "sample", "plateau_reference_percent", "service_percent", "absolute_difference" > comparisons
    difference_count = 0
    for (p = 1; p <= 2; p++) {
        phase = phases[p]
        for (i = 1; i <= count[phase]; i++) {
            key = phase SUBSEP i
            difference = absolute(aggregate[phase] - svc[key])
            printf "%s\t%d\t%.6f\t%.6f\t%.6f\n", phase, i, aggregate[phase], svc[key], difference > comparisons
            differences[++difference_count] = difference
        }
    }

    print "sample", "reference_percent", "service_percent" > all_core
    for (i = 1; i <= count["full"]; i++) {
        key = "full" SUBSEP i
        printf "%d\t%.6f\t%.6f\n", i, ref[key], svc[key] > all_core
    }

    for (i = 1; i <= difference_count; i++) {
        for (j = i + 1; j <= difference_count; j++) {
            if (differences[j] < differences[i]) {
                temporary = differences[i]
                differences[i] = differences[j]
                differences[j] = temporary
            }
        }
    }
    median = difference_count == 20 ? (differences[10] + differences[11]) / 2 : -1
    p95 = difference_count == 20 ? differences[19] : -1

    product_failed = 0
    if (!invalid && (median > 5 || p95 > 10))
        product_failed = 1
    if (!invalid) {
        for (i = 1; i <= count["full"]; i++) {
            if (svc["full" SUBSEP i] < 90) product_failed = 1
        }
    }

    classification = invalid ? "BLOCKED" : (product_failed ? "FAIL" : "PASS")
    reason = invalid ? invalid_reason : (product_failed ? "valid reference evidence did not meet product thresholds" : "all CPU methodology checks passed")
    printf "classification=%s\n", classification > summary
    printf "reason=%s\n", reason > summary
    printf "comparison_count=%d\n", difference_count > summary
    printf "idle_aggregate_reference=%.6f\n", aggregate["idle"] > summary
    printf "half_aggregate_reference=%.6f\n", aggregate["half"] > summary
    printf "full_aggregate_reference=%.6f\n", aggregate["full"] > summary
    printf "median_absolute_difference=%.6f\n", median > summary
    printf "p95_absolute_difference=%.6f\n", p95 > summary
    printf "median_limit=5\n" > summary
    printf "p95_limit=10\n" > summary
    printf "full_load_minimum=90\n" > summary
    printf "full_reference_minimum=%.6f\n", minimum["full"] > summary
    full_service_minimum = 101
    for (i = 1; i <= count["full"]; i++)
        if (svc["full" SUBSEP i] < full_service_minimum) full_service_minimum = svc["full" SUBSEP i]
    printf "full_service_minimum=%.6f\n", full_service_minimum > summary

    if (invalid) exit 2
    if (product_failed) exit 1
    exit 0
}
