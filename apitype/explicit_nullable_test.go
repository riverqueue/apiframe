package apitype

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplicitNullable_Validation(t *testing.T) {
	t.Parallel()

	validate := NewValidator()

	tests := []struct {
		name      string
		json      string
		wantValid bool
	}{
		{
			name:      "FieldOmittedValid",
			json:      "{}",
			wantValid: true,
		},
		{
			name:      "ExplicitNullValid",
			json:      `{"label":null}`,
			wantValid: true,
		},
		{
			name:      "EmptyStringInvalid",
			json:      `{"label":""}`,
			wantValid: false,
		},
		{
			name:      "ValidShortString",
			json:      `{"label":"a"}`,
			wantValid: true,
		},
		{
			name:      "ValidString",
			json:      `{"label":"test"}`,
			wantValid: true,
		},
		{
			name:      "StringTooLongInvalid",
			json:      `{"label":"` + strings.Repeat("a", 101) + `"}`,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var payload testPayload

			err := json.Unmarshal([]byte(tt.json), &payload)
			require.NoError(t, err)

			err = validate.Struct(payload)
			if tt.wantValid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestExtractExplicitNullableValueForValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		wantVal any
	}{
		{
			name:    "FieldNotSet",
			input:   ExplicitNullable[string]{Set: false},
			wantVal: nil,
		},
		{
			name:    "FieldExplicitlyNull",
			input:   ExplicitNullable[string]{Set: true, Value: nil},
			wantVal: nil,
		},
		{
			name:    "EmptyStringValue",
			input:   ExplicitNullable[string]{Set: true, Value: ptr("")},
			wantVal: ptr(""),
		},
		{
			name:    "NonEmptyStringValue",
			input:   ExplicitNullable[string]{Set: true, Value: ptr("test")},
			wantVal: ptr("test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			val := reflect.ValueOf(tt.input)

			got := ExtractExplicitNullableValueForValidation[string](val)
			if tt.wantVal == nil {
				require.Nil(t, got)
			} else {
				require.Equal(t, tt.wantVal, got)
			}
		})
	}
}

type testPayload struct {
	Label ExplicitNullable[string] `json:"label" validate:"omitempty,min=1,max=100"`
}

func TestExplicitNullable_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		want    ExplicitNullable[string]
		wantErr bool
	}{
		{
			name: "FieldOmitted",
			json: "{}",
			want: ExplicitNullable[string]{Set: false, Value: nil},
		},
		{
			name: "ExplicitNull",
			json: `{"label":null}`,
			want: ExplicitNullable[string]{Set: true, Value: nil},
		},
		{
			name: "EmptyString",
			json: `{"label":""}`,
			want: ExplicitNullable[string]{Set: true, Value: ptr("")},
		},
		{
			name: "NonEmptyString",
			json: `{"label":"test"}`,
			want: ExplicitNullable[string]{Set: true, Value: ptr("test")},
		},
		{
			name:    "InvalidJSON",
			json:    `{"label":}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got testPayload

			err := json.Unmarshal([]byte(tt.json), &got)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got.Label)
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
