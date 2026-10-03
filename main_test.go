package main

import (
	"fmt"
	"testing"
)

func Test_detectContentType(t *testing.T) {
	type args struct {
		filename string
		data     []byte
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "detect css",
			args: args{
				filename: "fonts-family.css",
				data:     nil,
			},
			want: "text/css; charset=utf-8",
		},
		{
			name: "detect woff2 font",
			args: args{
				filename: "some.woff2",
				data:     nil,
			},
			want: "font/woff2",
		},
		{
			name: "detect default type",
			args: args{
				filename: "some.html",
				data:     nil,
			},
			want: "text/html; charset=utf-8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectContentType(tt.args.filename, tt.args.data)
			fmt.Println(got)
			if got != tt.want {
				t.Errorf("detectContentType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateRequiredInputs(t *testing.T) {
	tests := []struct {
		name     string
		bucket   string
		operator string
		password string
		wantErr  string
	}{
		{
			name:     "all required inputs present",
			bucket:   "bucket",
			operator: "operator",
			password: "password",
		},
		{
			name:     "bucket missing",
			operator: "operator",
			password: "password",
			wantErr:  "missing required input(s): bucket",
		},
		{
			name:     "operator missing",
			bucket:   "bucket",
			password: "password",
			wantErr:  "missing required input(s): operator",
		},
		{
			name:     "password missing",
			bucket:   "bucket",
			operator: "operator",
			wantErr:  "missing required input(s): password",
		},
		{
			name:    "all required inputs missing",
			wantErr: "missing required input(s): bucket, operator, password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequiredInputs(tt.bucket, tt.operator, tt.password)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRequiredInputs() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateRequiredInputs() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("validateRequiredInputs() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
