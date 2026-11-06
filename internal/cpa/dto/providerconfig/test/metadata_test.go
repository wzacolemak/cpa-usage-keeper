package providerconfig_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
)

func TestProviderGroupProjectionInheritsAndPreservesExplicitOverrides(t *testing.T) {
	body := `[{"name":"routing group","base-url":"https://example.invalid/v1","prefix":"team","priority":8,"excluded-models":["model-x","*"],"keys":[{"api-key":"first","auth_index":"shared","priority":null,"excluded-models":null},{"api-key":"second","auth_index":"second","priority":0,"prefix":"","excluded-models":[]},{"api-key":"third","auth_index":"third","name":"member name"}]}]`
	var document providerconfig.Document
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		t.Fatal(err)
	}
	keys, err := document.ProviderKeys("")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("keys = %#v", keys)
	}
	if keys[0].Priority == nil || *keys[0].Priority != 8 || keys[0].Prefix != "team" || !*keys[0].Disabled {
		t.Fatalf("inherited key = %#v", keys[0])
	}
	if keys[1].Priority == nil || *keys[1].Priority != 0 || keys[1].Prefix != "" || *keys[1].Disabled || !reflect.DeepEqual(keys[1].ExcludedModels, []string{}) {
		t.Fatalf("explicit overrides = %#v", keys[1])
	}
	if keys[0].Name != "" || keys[2].Name != "member name" || keys[2].BaseURL != "https://example.invalid/v1" {
		t.Fatalf("name/base URL = %#v", keys)
	}
}

func TestMetaDefaultBaseURLOnlyAppearsInProjection(t *testing.T) {
	var document providerconfig.Document
	if err := json.Unmarshal([]byte(`[{"keys":[{"api-key":"meta-key","auth_index":"meta-auth"}]}]`), &document); err != nil {
		t.Fatal(err)
	}
	keys, err := document.ProviderKeys("https://api.meta.ai/v1")
	if err != nil || keys[0].BaseURL != "https://api.meta.ai/v1" {
		t.Fatalf("projection = %#v, err = %v", keys, err)
	}
	if _, exists := document.Groups[0].Fields["base-url"]; exists {
		t.Fatal("default base URL changed raw config")
	}
}

func TestOpenAIGroupProjectionKeepsGroupPriorityAndDisabled(t *testing.T) {
	var document providerconfig.Document
	if err := json.Unmarshal([]byte(`[{"name":"router","priority":4,"disabled":false,"excluded-models":["*"],"keys":[{"api-key":"first","auth_index":"a","priority":9,"proxy-url":null,"weight":2},{"api-key":"second","auth_index":"b"}]}]`), &document); err != nil {
		t.Fatal(err)
	}
	groups, err := document.OpenAIProviders()
	if err != nil {
		t.Fatal(err)
	}
	group := groups[0]
	if *group.Priority != 4 || *group.Disabled || len(group.APIKeyEntries) != 2 || group.APIKeyEntries[1].AuthIndex != "b" {
		t.Fatalf("group = %#v", group)
	}
}

func TestProviderDocumentRejectsMalformedGroupAndMemberStructures(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[null]`, `[{"keys":null}]`, `[{"keys":{}}]`, `[{"keys":[null]}]`, `[{"keys":["key"]}]`} {
		var document providerconfig.Document
		if err := json.Unmarshal([]byte(body), &document); err == nil {
			t.Fatalf("decoded malformed structure: %s", body)
		}
	}
}

// TestProviderKeyConfigUnmarshalsDisplayName 锁定标准 API Key entry 的 display-name 兼容字段来源。
func TestProviderKeyConfigUnmarshalsDisplayName(t *testing.T) {
	var cfg providerconfig.ProviderKeyConfig
	if err := json.Unmarshal([]byte(`{"api-key":"provider-key","display-name":"Primary Gemini"}`), &cfg); err != nil {
		t.Fatalf("unmarshal provider key config: %v", err)
	}
	if cfg.DisplayName != "Primary Gemini" {
		t.Fatalf("DisplayName = %q, want Primary Gemini", cfg.DisplayName)
	}
}

// TestOpenAICompatibilityConfigUnmarshalsDisplayNameSeparatelyFromName 锁定 display-name 与路由 name 各自独立解码。
func TestOpenAICompatibilityConfigUnmarshalsDisplayNameSeparatelyFromName(t *testing.T) {
	var cfg providerconfig.OpenAICompatibilityConfig
	if err := json.Unmarshal([]byte(`{"name":"route-id","display-name":"Friendly upstream"}`), &cfg); err != nil {
		t.Fatalf("unmarshal openai compatibility config: %v", err)
	}
	if cfg.Name != "route-id" || cfg.DisplayName != "Friendly upstream" {
		t.Fatalf("unexpected names: %#v", cfg)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
