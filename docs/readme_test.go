package docs

import (
	"bufio"
	"fmt"
	"io"
	"testing"

	"github.com/NetSys/quilt/stitch"
	"github.com/NetSys/quilt/util"
)

func TestReadme(t *testing.T) {
	t.Skip("Vivian will covert the README snippets")
	f, err := util.Open("../README.md")
	if err != nil {
		t.Errorf("Failed to open README: %s", err.Error())
		return
	}
	defer f.Close()

	r := bufio.NewReader(f)

	start := "<!-- BEGIN CODE -->\n"
	end := "<!-- END CODE -->\n"
	var code string
	recording := false

	for {
		line, err := r.ReadString('\n')

		if err == io.EOF {
			if recording {
				fmt.Printf("Unbalanced code blocks.")
				return
			}
			break
		}

		if err != nil {
			t.Errorf("Failed to read README: %s", err.Error())
			return
		}

		if line == start {
			if recording {
				t.Errorf("Unbalanced code blocks.")
				return
			}
			recording = true
			continue
		}

		if line == end {
			if !recording {
				t.Errorf("Unbalanced code blocks.")
				return
			}
			recording = false
		}

		if recording {
			code += fmt.Sprintf("%s", line)
		}
	}

	if err = checkConfig(code); err != nil {
		t.Errorf(err.Error())
	}
}

func checkConfig(content string) error {
	_, err := stitch.New(content, stitch.DefaultImportGetter)
	if err != nil {
		return err
	}
	return nil
}
