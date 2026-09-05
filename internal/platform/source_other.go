//go:build !linux

package platform

import (
	"context"
	"io/fs"
)

// NewSource preserves development builds on unsupported operating systems.
// Every observation remains unavailable; Linux is the production runtime.
func NewSource() Source { return unsupportedSource{} }

type unsupportedSource struct{}

func (unsupportedSource) Hostname() (string, error)             { return "", fs.ErrNotExist }
func (unsupportedSource) Uptime() ([]byte, error)               { return nil, fs.ErrNotExist }
func (unsupportedSource) CPUOnline() ([]byte, error)            { return nil, fs.ErrNotExist }
func (unsupportedSource) LoadAverage() ([]byte, error)          { return nil, fs.ErrNotExist }
func (unsupportedSource) MemoryInfo() ([]byte, error)           { return nil, fs.ErrNotExist }
func (unsupportedSource) RootFilesystem() (Filesystem, error)   { return Filesystem{}, fs.ErrNotExist }
func (unsupportedSource) Links(context.Context) ([]Link, error) { return nil, fs.ErrNotExist }
