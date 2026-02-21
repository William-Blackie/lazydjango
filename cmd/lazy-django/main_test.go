package main

import "testing"

func TestParseOptionsVersion(t *testing.T) {
	opts, err := parseOptions([]string{"--version"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !opts.showVersion {
		t.Fatal("expected showVersion=true")
	}
}

func TestParseOptionsDoctorStrictRequiresDoctor(t *testing.T) {
	_, err := parseOptions([]string{"--doctor-strict"})
	if err == nil {
		t.Fatal("expected error for --doctor-strict without --doctor")
	}
}

func TestParseOptionsProjectEqualsSyntax(t *testing.T) {
	opts, err := parseOptions([]string{"--doctor", "--project=/tmp/demo"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if opts.projectDir != "/tmp/demo" {
		t.Fatalf("expected projectDir=/tmp/demo, got %q", opts.projectDir)
	}
}

func TestVersionString(t *testing.T) {
	oldVersion := version
	oldCommit := commit
	oldDate := date
	oldBuiltBy := builtBy
	t.Cleanup(func() {
		version = oldVersion
		commit = oldCommit
		date = oldDate
		builtBy = oldBuiltBy
	})

	version = "1.2.3"
	commit = "abc123"
	date = "2026-02-21T00:00:00Z"
	builtBy = "goreleaser"

	got := versionString()
	want := "lazy-django 1.2.3 (commit=abc123 date=2026-02-21T00:00:00Z builtBy=goreleaser)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
