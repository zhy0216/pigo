//go:build !memprofile

package main

func setupMemProfile() func() {
	return func() {}
}
