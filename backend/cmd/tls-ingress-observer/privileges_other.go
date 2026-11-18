//go:build !linux

package main

import "errors"

func dropObserverPrivileges() error {
	return errors.New("authenticated ingress observer requires Linux privilege isolation")
}
