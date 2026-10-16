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

func TestEvaluatePrincipalAndDeny(t *testing.T) {
	doc := `{
		"Version":"2012-10-17",
		"Statement":[
			{"Effect":"Allow","Principal":{"AWS":["AKIAALLOWED"]},"Action":"s3:GetObject","Resource":"arn:aws:s3:::b/*"},
			{"Effect":"Deny","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::b/secret.txt"}
		]
	}`
	if !Evaluate(doc, "s3:GetObject", "arn:aws:s3:::b/file.txt", "AKIAALLOWED") {
		t.Fatal("allowed key should read")
	}
	if Evaluate(doc, "s3:GetObject", "arn:aws:s3:::b/file.txt", "AKIAOTHER") {
		t.Fatal("other key must not read")
	}
	if Evaluate(doc, "s3:GetObject", "arn:aws:s3:::b/secret.txt", "AKIAALLOWED") {
		t.Fatal("deny must override allow")
	}
}

func TestValidateRejectsUnknownCondition(t *testing.T) {
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::b/*","Condition":{"StringEquals":{"s3:prefix":"a"}}}]}`
	if err := Validate(doc); err == nil {
		t.Fatal("expected unsupported condition")
	}
}
