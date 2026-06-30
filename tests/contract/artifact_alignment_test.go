package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeSourceHonorsStackBoundary(t *testing.T) {
	for _, root := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(b)
			for _, forbidden := range []string{
				"github.com/spf13/viper",
				"github.com/fatih/color",
				"github.com/sirupsen/logrus",
				"go.uber.org/zap",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s violates runtime stack boundary with %q", path, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
