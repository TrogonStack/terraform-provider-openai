package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func configureProvider(t *testing.T) *provider.ConfigureResponse {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema has errors: %v", schemaResp.Diagnostics)
	}

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{})

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

func TestProviderMetadata(t *testing.T) {
	p := New("1.2.3")()
	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, resp)
	if resp.TypeName != "example" {
		t.Fatalf("expected type name example, got %s", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %s", resp.Version)
	}
}

func TestProviderConfigure(t *testing.T) {
	resp := configureProvider(t)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected configure to succeed, got %v", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*http.Client)
	if !ok || client == nil {
		t.Fatalf("expected an *http.Client, got %#v", resp.ResourceData)
	}
	if resp.DataSourceData != resp.ResourceData {
		t.Fatal("expected data sources and resources to share the client")
	}
}

func TestProviderResourcesAndDataSources(t *testing.T) {
	p := New("test")()
	if got := p.Resources(context.Background()); len(got) != 0 {
		t.Fatalf("expected no resources yet, got %d", len(got))
	}
	if got := p.DataSources(context.Background()); len(got) != 0 {
		t.Fatalf("expected no data sources yet, got %d", len(got))
	}
}
