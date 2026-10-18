package hisarkit

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSharedSentencesNameNoPlatform keeps the Mac's and the iPhone's words out of the sentences every app shows. A
// sentence for one platform goes in a file with a //go:build line, as platform_other.go, and the biometric check is
// named by app.Config.BiometricName.
func TestSharedSentencesNameNoPlatform(t *testing.T) {
	platformWords := []string{"iPhone", "Face ID", "Touch ID"}
	fileSet := token.NewFileSet()
	for _, root := range []string{".", "../../core"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			if hasBuildConstraint(file) {
				return nil
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, isLiteral := node.(*ast.BasicLit)
				if !isLiteral || literal.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, word := range platformWords {
					if strings.Contains(text, word) {
						t.Errorf("%s: %q says %q in a sentence every platform shows", fileSet.Position(literal.Pos()), text, word)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func hasBuildConstraint(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			return false
		}
		for _, comment := range group.List {
			if constraint.IsGoBuild(comment.Text) {
				return true
			}
		}
	}
	return false
}
