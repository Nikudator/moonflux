package user

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileUpdateInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "empty update",
			input: `{}`,
		},
		{
			name:  "valid nickname",
			input: `{"nickname":"new-nickname"}`,
		},
		{
			name:  "clear patronymic",
			input: `{"patronymic":null}`,
		},
		{
			name:  "clear birth date",
			input: `{"birth_date":null}`,
		},
		{
			name:  "valid about",
			input: `{"about":"Hello 🌙"}`,
		},
		{
			name:    "null nickname",
			input:   `{"nickname":null}`,
			wantErr: true,
		},
		{
			name:    "blank nickname",
			input:   `{"nickname":"   "}`,
			wantErr: true,
		},
		{
			name:    "nickname too long",
			input:   `{"nickname":"` + strings.Repeat("a", 41) + `"}`,
			wantErr: true,
		},
		{
			name:    "null first name",
			input:   `{"first_name":null}`,
			wantErr: true,
		},
		{
			name:    "blank last name",
			input:   `{"last_name":" "}`,
			wantErr: true,
		},
		{
			name:    "patronymic too long",
			input:   `{"patronymic":"` + strings.Repeat("a", 101) + `"}`,
			wantErr: true,
		},
		{
			name:    "about too long",
			input:   `{"about":"` + repeatRune('я', 2001) + `"}`,
			wantErr: true,
		},
		{
			name:    "null about",
			input:   `{"about":null}`,
			wantErr: true,
		},
		{
			name:    "zero birth date",
			input:   `{"birth_date":"0001-01-01T00:00:00Z"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input ProfileUpdateInput
			if err := json.Unmarshal([]byte(tt.input), &input); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			err := input.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProfileUpdateOptionalDistinguishesOmittedAndNull(t *testing.T) {
	var omitted ProfileUpdateInput
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatal(err)
	}

	if omitted.Patronymic.Present {
		t.Fatal("omitted patronymic should not be present")
	}

	var cleared ProfileUpdateInput
	if err := json.Unmarshal([]byte(`{"patronymic":null}`), &cleared); err != nil {
		t.Fatal(err)
	}

	if !cleared.Patronymic.Present || !cleared.Patronymic.Null {
		t.Fatal("explicit null should be present and null")
	}

	var supplied ProfileUpdateInput
	if err := json.Unmarshal([]byte(`{"patronymic":"Ivanovich"}`), &supplied); err != nil {
		t.Fatal(err)
	}

	if !supplied.Patronymic.Present ||
		supplied.Patronymic.Null ||
		supplied.Patronymic.Value != "Ivanovich" {
		t.Fatal("supplied value was not decoded correctly")
	}
}

func TestProfileUpdateBirthDate(t *testing.T) {
	var input ProfileUpdateInput
	if err := json.Unmarshal(
		[]byte(`{"birth_date":"2000-01-02T00:00:00Z"}`),
		&input,
	); err != nil {
		t.Fatal(err)
	}

	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if input.BirthDate.Value.Year() != 2000 {
		t.Fatalf("unexpected birth date: %v", input.BirthDate.Value)
	}
}

func repeatRune(r rune, count int) string {
	out := make([]rune, count)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
