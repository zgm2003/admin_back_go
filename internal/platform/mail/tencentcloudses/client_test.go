package tencentcloudses

import "testing"

func TestBuildFromEmailAddress(t *testing.T) {
	if got := BuildFromEmailAddress("noreply@example.com", "Admin"); got != "Admin <noreply@example.com>" {
		t.Fatalf("unexpected from address: %q", got)
	}
	if got := BuildFromEmailAddress("noreply@example.com", ""); got != "noreply@example.com" {
		t.Fatalf("unexpected bare from address: %q", got)
	}
}

func TestTemplateDataJSONIsStable(t *testing.T) {
	got, err := TemplateDataJSON(map[string]string{"ttl_minutes": "5", "code": "123456", "app_name": "admin_go"})
	if err != nil {
		t.Fatalf("TemplateDataJSON returned error: %v", err)
	}
	if got != `{"app_name":"admin_go","code":"123456","ttl_minutes":"5"}` {
		t.Fatalf("unexpected template data: %s", got)
	}
}
