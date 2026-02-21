//go:build !debug

package main

func setupMemProfile() func() {
	return func() {}
}
