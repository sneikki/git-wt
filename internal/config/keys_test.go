package config

import "testing"

func TestPostCreate_RegisteredAndNotTemplated(t *testing.T) {
	if Find("postCreate") != PostCreate || Find("wt.postCreate") != PostCreate {
		t.Fatal("postCreate not found in the key registry")
	}
	if PostCreate.Templated {
		t.Fatal("postCreate must not be templated: shell $ must pass through")
	}
	if err := Validate(PostCreate, "bun install && echo $HOME ${repo}"); err != nil {
		t.Fatalf("Validate rejected a shell command: %v", err)
	}
}
