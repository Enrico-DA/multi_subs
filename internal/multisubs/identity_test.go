package multisubs

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProductIdentityInterlock(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate identity test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	checker := filepath.Join(root, "scripts", "ci", "check_identity.py")
	command := exec.Command("python3", checker, root)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("product identity interlock failed: %v\n%s", err, output)
	}
}
