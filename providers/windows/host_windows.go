// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package windows

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/joeshaw/multierror"

	stdwindows "golang.org/x/sys/windows"

	windows "github.com/elastic/go-windows"

	"github.com/elastic/go-sysinfo/internal/registry"
	"github.com/elastic/go-sysinfo/providers/shared"
	"github.com/elastic/go-sysinfo/types"
)

func init() {
	registry.Register(windowsSystem{})
}

type windowsSystem struct{}

func (s windowsSystem) Host() (types.Host, error) {
	return newHost()
}

type host struct {
	info types.HostInfo
}

func (h *host) Info() types.HostInfo {
	return h.info
}

func (h *host) CPUTime() (types.CPUTimes, error) {
	idle, kernel, user, err := windows.GetSystemTimes()
	if err != nil {
		return types.CPUTimes{}, err
	}

	return types.CPUTimes{
		System: kernel,
		User:   user,
		Idle:   idle,
	}, nil
}

func (h *host) Memory() (*types.HostMemoryInfo, error) {
	mem, err := windows.GlobalMemoryStatusEx()
	if err != nil {
		return nil, err
	}

	return &types.HostMemoryInfo{
		Total:        mem.TotalPhys,
		Used:         mem.TotalPhys - mem.AvailPhys,
		Free:         mem.AvailPhys,
		Available:    mem.AvailPhys,
		VirtualTotal: mem.TotalPageFile,
		VirtualUsed:  mem.TotalPageFile - mem.AvailPageFile,
		VirtualFree:  mem.AvailPageFile,
	}, nil
}

func newHost() (*host, error) {
	h := &host{}
	r := &reader{}
	r.architecture(h)
	r.bootTime(h)
	r.hostname(h)
	r.fqdn(h)
	r.network(h)
	r.kernelVersion(h)
	r.os(h)
	r.time(h)
	r.uniqueID(h)
	return h, r.Err()
}

type reader struct {
	errs []error
}

func (r *reader) addErr(err error) bool {
	if err != nil {
		if !errors.Is(err, types.ErrNotImplemented) {
			r.errs = append(r.errs, err)
		}
		return true
	}
	return false
}

func (r *reader) Err() error {
	if len(r.errs) > 0 {
		return &multierror.MultiError{Errors: r.errs}
	}
	return nil
}

func (r *reader) architecture(h *host) {
	v, err := Architecture()
	if r.addErr(err) {
		return
	}
	h.info.Architecture = v
}

func (r *reader) bootTime(h *host) {
	v, err := BootTime()
	if r.addErr(err) {
		return
	}
	h.info.BootTime = v
}

func (r *reader) hostname(h *host) {
	v, err := os.Hostname()
	if r.addErr(err) {
		return
	}
	h.info.Hostname = v
}

func (r *reader) fqdn(h *host) {
	fqdn, err := getComputerNameEx(
		stdwindows.ComputerNamePhysicalDnsFullyQualified)
	if err != nil {
		r.addErr(fmt.Errorf("could not get windows FQDN: %s", err))
		return
	}

	h.info.FQDN = strings.TrimSuffix(fqdn, ".")
}

func getComputerNameEx(name uint32) (string, error) {
	size := uint32(64)

	for {
		buff := make([]uint16, size)
		err := stdwindows.GetComputerNameEx(
			name, &buff[0], &size)
		if err == nil {
			return syscall.UTF16ToString(buff[:size]), nil
		}

		// ERROR_MORE_DATA means buff is too small and size is set to the
		// number of bytes needed to store the FQDN. For details, see
		// https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-getcomputernameexw#return-value
		if errors.Is(err, syscall.ERROR_MORE_DATA) {
			// Safeguard to avoid an infinite loop.
			if size <= uint32(len(buff)) {
				return "", fmt.Errorf(
					"windows.GetComputerNameEx returned ERROR_MORE_DATA, " +
						"but data size should fit into buffer")
			} else {
				// Grow the buffer and try again.
				buff = make([]uint16, size)
				continue
			}
		}

		return "", fmt.Errorf("could not get windows FQDN: could not get windows.ComputerNamePhysicalDnsFullyQualified: %w", err)
	}
}

func (r *reader) network(h *host) {
	ips, macs, err := shared.Network()
	if r.addErr(err) {
		return
	}
	h.info.IPs = ips
	h.info.MACs = macs
}

func (r *reader) kernelVersion(h *host) {
	v, err := KernelVersion()
	if r.addErr(err) {
		return
	}
	h.info.KernelVersion = v
}

func (r *reader) os(h *host) {
	v, err := OperatingSystem()
	if r.addErr(err) {
		return
	}
	h.info.OS = v
}

func (r *reader) time(h *host) {
	h.info.Timezone, h.info.TimezoneOffsetSec = time.Now().Zone()
}

func (r *reader) uniqueID(h *host) {
	v, err := MachineID()
	if r.addErr(err) {
		return
	}
	h.info.UniqueID = v
}
