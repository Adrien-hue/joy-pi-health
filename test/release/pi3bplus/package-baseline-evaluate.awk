BEGIN {
    FS = "\t"
    required_count = split("package_query_absent selection_absent payload_absent dpkg_info_absent dpkg_state_absent unit_not_found unit_not_enabled systemd_state_absent helper_system_absent helper_user_absent acl_absent identity_valid process_absent port_available policy_recorded", required, " ")
    for (i = 1; i <= required_count; i++) {
        allowed[required[i]] = 1
    }
    malformed = 0
}

NF < 2 {
    malformed = 1
    next
}

{
    name = $1
    status = $2
    if (!(name in allowed) || name in seen || (status != "PASS" && status != "BLOCKED")) {
        malformed = 1
        next
    }
    seen[name] = 1
    result[name] = status
}

END {
    blocked = malformed
    for (i = 1; i <= required_count; i++) {
        name = required[i]
        if (!(name in seen) || result[name] != "PASS") {
            blocked = 1
        }
    }

    if (blocked) {
        print "classification=BLOCKED"
        exit 1
    }
    print "classification=PASS"
}
