package contract

import "testing"

func TestClassifyPartialAndBroken(t *testing.T) {
	subset := map[string]any{
		"api": "v1",
		"features": map[string]any{
			"auth.password": float64(1),
			"future.widget": float64(2),
		},
	}
	profile, features := Classify(0, nil, 200, subset, "1.5.0", false)
	if profile != "features" || features["auth.password"] != 1 || features["sharing.links"] != 0 {
		t.Fatalf("partial features: %s %#v", profile, features)
	}
	if features["future.widget"] != 2 {
		t.Fatal("unknown keys are kept and do not fail classification")
	}

	plain := map[string]any{"api": "v1", "server_version": "1.6.1", "login_methods": []any{"password"}}
	profile, features = Classify(200, plain, 0, nil, "1.6.1", false)
	if profile != "1.6.0" || features["sharing.policy"] != 1 || features["errors.codes"] != 0 {
		t.Fatalf("body without features uses healthz: %s %#v", profile, features)
	}

	profile, features = Classify(200, map[string]any{}, 0, nil, "1.5.0", false)
	if profile != "1.5.0" || features["objects.multipart"] != 1 || features["auth.step_up"] != 0 {
		t.Fatalf("unparsed capabilities use healthz: %s %#v", profile, features)
	}

	profile, _ = Classify(404, map[string]any{"error": "not found"}, 0, nil, "dev", false)
	if profile != "1.5.0" {
		t.Fatalf("dev without kms is 1.5.0, got %s", profile)
	}
	profile, features = Classify(404, nil, 0, nil, "dev", true)
	if profile != "1.6.0" || features["encryption.kms"] != 1 {
		t.Fatalf("dev with kms is 1.6.0, got %s %#v", profile, features)
	}
	profile, _ = Classify(404, nil, 0, nil, "1.4.1", false)
	if profile != "rejected" {
		t.Fatalf("1.4.1 is rejected, got %s", profile)
	}
}
