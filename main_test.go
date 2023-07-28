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
