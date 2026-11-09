// Package hwtest holds opt-in tests against real cameras. They skip unless
// PUNCHLINE_HW=1, so CI never needs hardware:
//
//	PUNCHLINE_HW=1 go test -v -count=1 ./internal/camera/hwtest
//
// With a supported camera plugged in, each one found is opened, set to every
// zoom level it offers, read back, timed, and left at 1x.
package hwtest
