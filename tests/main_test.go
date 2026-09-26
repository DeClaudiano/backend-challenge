package recovery

import (
	"os"
	"syscall"
	"testing"
)

func TestMain(m *testing.M) {
	lockPath := "/tmp/backend-challenge-postgres-integration.lock"

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		panic(err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	os.Exit(m.Run())
}
