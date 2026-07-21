package main

import (
	"reflect"
	"testing"
)

func TestWatchNamespacesFromEnv(t *testing.T) {
	const paymentsNamespace = "payments"

	tests := []struct {
		name string
		env  string
		want []string
	}{
		{name: "unset", env: "", want: nil},
		{name: "single namespace", env: paymentsNamespace, want: []string{paymentsNamespace}},
		{name: "trims spaces", env: " payments , checkout ", want: []string{paymentsNamespace, "checkout"}},
		{
			name: "skips blanks and duplicates",
			env:  "payments,,checkout,payments",
			want: []string{paymentsNamespace, "checkout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WATCH_NAMESPACE", tt.env)

			got := watchNamespacesFromEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("watchNamespacesFromEnv() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
