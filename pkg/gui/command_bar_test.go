package gui

import "testing"

func TestIsHelpCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "help", want: true},
		{input: "HELP", want: true},
		{input: ":help", want: true},
		{input: " h ", want: true},
		{input: "?", want: true},
		{input: "check", want: false},
		{input: "", want: false},
	}

	for _, tc := range tests {
		got := isHelpCommand(tc.input)
		if got != tc.want {
			t.Fatalf("isHelpCommand(%q)=%v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestOpenHelpModal(t *testing.T) {
	gui := &Gui{currentWindow: MenuWindow}
	if err := gui.openHelpModal(); err != nil {
		t.Fatalf("openHelpModal returned error: %v", err)
	}
	if !gui.isModalOpen {
		t.Fatal("expected help modal to be open")
	}
	if gui.modalType != "help" {
		t.Fatalf("expected modalType help, got %q", gui.modalType)
	}
	if gui.modalTitle != "Help" {
		t.Fatalf("expected modalTitle Help, got %q", gui.modalTitle)
	}
	if gui.modalMessage == "" {
		t.Fatal("expected help modal message to be populated")
	}
}
