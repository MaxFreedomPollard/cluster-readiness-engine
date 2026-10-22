// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package docspolicy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// freezeScript builds the content every registered Fern version is served
// from, so what it writes is what the published site shows.
const freezeScript = "../../hack/fern-freeze-versions.sh"

// TestFrozenDocsEscapeOnlyText runs the real freeze script over a tagged
// fixture and reads back the pages it would publish.
//
// The script escapes {, } and < because Fern parses every page as MDX, where
// they would open an expression or a tag. Escaping one that is not text fails
// nothing -- the page still builds -- so the damage only shows on the site: a
// fenced block indented under a list item printed &lt;new-version> in a
// command readers copy, and every <Note> and <Warning> callout printed its
// tags around plain text.
func TestFrozenDocsEscapeOnlyText(t *testing.T) {
	// Pages with nothing to escape. Each must come out exactly as it went in.
	const (
		fence       = "```yaml\nkey: {a: <b>}\n```"
		listFence   = "1. Upgrade:\n\n   ```bash\n   helm upgrade nvcre --version <new-version>\n   ```"
		nestedFence = "- Catalog:\n  - Render:\n\n    ~~~\n    {{ lib \"nccl/env.yaml\" . }}\n    ~~~"
		callout     = "<Callout intent=\"warning\">\nRead this first.\n</Callout>"
	)

	cases := []struct {
		name string
		page string
		want string
	}{
		{name: "text", page: "Use {a} if x < 3; `{b} <c>` is code.", want: "Use \\{a\\} if x &lt; 3; `{b} <c>` is code."},
		{name: "placeholder in text", page: "Replace <TAG> with the tag.", want: "Replace &lt;TAG> with the tag."},
		{name: "fenced block", page: fence, want: fence},
		{name: "fenced block in a list item", page: listFence, want: listFence},
		{name: "tilde fence in a nested list item", page: nestedFence, want: nestedFence},
		{name: "callout", page: "<Warning>\nKeep x < 3.\n</Warning>", want: "<Warning>\nKeep x &lt; 3.\n</Warning>"},
		{name: "callout with attributes", page: callout, want: callout},
		{name: "HTML comment", page: "<!-- SPDX-License-Identifier: Apache-2.0 -->\n# Title", want: "# Title"},
		{name: "HTML comment between paragraphs", page: "First.\n<!-- note -->\nSecond.", want: "First.\n\nSecond."},
	}

	const tag = "v0.0.1"
	repo := t.TempDir()

	write := func(rel, content string) {
		t.Helper()
		file := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("create the directory for %s: %v", rel, err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = repo
		// The scratch repository must not pick up signing, hooks or an
		// identity from the configuration of whoever runs the tests.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=docspolicy", "GIT_AUTHOR_EMAIL=docspolicy@example.com",
			"GIT_COMMITTER_NAME=docspolicy", "GIT_COMMITTER_EMAIL=docspolicy@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}

	script, err := os.ReadFile(freezeScript)
	if err != nil {
		t.Fatalf("read %s: %v", freezeScript, err)
	}
	write("hack/fern-freeze-versions.sh", string(script))
	for i, c := range cases {
		write(fmt.Sprintf("docs/%d.md", i), c.page+"\n")
	}
	run("git", "init", "--quiet")
	run("git", "add", "docs")
	run("git", "commit", "--quiet", "--message", "docs")
	run("git", "tag", tag)
	// Registered the way the publish workflow registers a release. The script
	// reads only the file name; the navigation it holds does not matter here.
	write("fern/versions/"+tag+".yml", "navigation: []\n")
	run("bash", "hack/fern-freeze-versions.sh")

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := os.ReadFile(filepath.Join(repo, "fern", "versions", tag+"-content", fmt.Sprintf("%d.md", i)))
			if err != nil {
				t.Fatalf("read the frozen page: %v", err)
			}
			if strings.TrimSpace(string(got)) != c.want {
				t.Errorf("frozen page differs from what Fern should publish\n got: %q\nwant: %q",
					strings.TrimSpace(string(got)), c.want)
			}
		})
	}
}
