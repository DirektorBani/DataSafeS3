package api

import (
	"errors"
	"testing"

	smithy "github.com/aws/smithy-go"
)

func TestIsS3NotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"nosuchkey", &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}, true},
		{"notfound", &smithy.GenericAPIError{Code: "NotFound", Message: "missing"}, true},
		{"nosuchbucket", &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "missing"}, true},
		{"other", &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isS3NotFound(tc.err); got != tc.want {
				t.Fatalf("isS3NotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestGatewayConnNotFoundErr(t *testing.T) {
	err := gatewayConnNotFoundErr("dead-conn")
	if err == nil || err.Error() == "not found" {
		t.Fatalf("expected descriptive error, got %v", err)
	}
	if !errors.Is(err, err) {
		t.Fatal("error should be non-nil")
	}
}
