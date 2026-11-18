//go:build linux

package main

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const observerRuntimeUID = 65534

// dropObserverPrivileges 只允许进程以 root 打开 AF_PACKET，随后永久降为 nobody。
// setuid 后 Linux 会清除有效 capability，已打开的受限 packet socket仍可继续读取。
func dropObserverPrivileges() error {
	if os.Geteuid() != 0 {
		return errors.New("observer must start as root only long enough to open the packet socket")
	}
	if err := syscall.Setgroups([]int{}); err != nil {
		return err
	}
	if err := syscall.Setgid(observerRuntimeUID); err != nil {
		return err
	}
	if err := syscall.Setuid(observerRuntimeUID); err != nil {
		return err
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		return errors.New("observer privilege drop did not take effect")
	}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	capabilities := [2]unix.CapUserData{}
	if err := unix.Capset(&header, &capabilities[0]); err != nil {
		return err
	}
	if err := unix.Capget(&header, &capabilities[0]); err != nil {
		return err
	}
	for _, capability := range capabilities {
		if capability.Effective != 0 || capability.Permitted != 0 || capability.Inheritable != 0 {
			return errors.New("observer retained capabilities after privilege drop")
		}
	}
	return nil
}
