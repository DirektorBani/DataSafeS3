package policy

import "testing"

func TestEvaluateEmptyPolicyAllowsAuthenticated(t *testing.T) {
	if !Evaluate("", "s3:GetObject", "arn:aws:s3:::b/k", "user") {
		t.Fatal("expected allow for authenticated principal")
	}
	if Evaluate("", "s3:GetObject", "arn:aws:s3:::b/k", "") {
		t.Fatal("expected deny for empty principal")
	}
}

func TestEvaluateAllowStatement(t *testing.T) {
	doc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":"*",
			"Action":["s3:GetObject"],
			"Resource":["arn:aws:s3:::public-bucket/*"]
		}]
	}`
	if !Evaluate(doc, "s3:GetObject", "arn:aws:s3:::public-bucket/file.txt", "reader") {
		t.Fatal("expected allow")
	}
	if Evaluate(doc, "s3:PutObject", "arn:aws:s3:::public-bucket/file.txt", "reader") {
		t.Fatal("expected deny for put")
	}
}

func TestEvaluateWildcardAction(t *testing.T) {
	doc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Principal":"*",
			"Action":["s3:*"],
			"Resource":["arn:aws:s3:::admin/*"]
		}]
	}`
	if !Evaluate(doc, "s3:DeleteObject", "arn:aws:s3:::admin/x", "u") {
		t.Fatal("expected allow via s3:*")
	}
}
