// Package provider is the Terraform and OpenTofu provider for the healthchecks.io
// Management API v3.
package provider

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// envKeyVar and envURLVar are the environment variables that the provider reads when the
// configuration does not set api_key or api_url.
const (
	envKeyVar = "HEALTHCHECKSIO_API_KEY"
	envURLVar = "HEALTHCHECKSIO_API_URL"
)

var (
	_ provider.ProviderWithActions       = (*hcProvider)(nil)
	_ provider.ProviderWithListResources = (*hcProvider)(nil)
)

// New gives a function that makes the provider. version is the release version, or
// "dev" for a local build.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &hcProvider{version: version} }
}

type hcProvider struct {
	version string
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

func (p *hcProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "healthchecksio"
	resp.Version = p.version
}

func (p *hcProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages checks in one healthchecks.io project with the Management API v3. " +
			"Each project has its own API keys. For more than one project, use one provider block for each project, with an `alias`.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "The API key of the project. Get it on the **Project Settings** page. " +
					"A read-only key is sufficient for the check, checks, badges and flips data sources. " +
					"The resource and the actions need a read-write key. If you do not set it, the provider uses `" + envKeyVar + "`.",
				Optional:  true,
				Sensitive: true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "The API root. The path must end in `/api/v3/`. " +
					"For a self-hosted server, use `<SITE_ROOT>/api/v3/`. If you do not set it, the provider uses `" +
					envURLVar + "`, then `" + client.DefaultURL + "`.",
				Optional: true,
			},
		},
	}
}

// Configure makes the API client. If a value in the provider block is unknown during the
// plan, Configure defers the resources when the client allows it.
func (p *hcProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.APIKey.IsUnknown() || cfg.APIURL.IsUnknown() {
		if req.ClientCapabilities.DeferralAllowed {
			resp.Deferred = &provider.Deferred{Reason: provider.DeferredReasonProviderConfigUnknown}
			return
		}
		resp.Diagnostics.AddError("Unknown provider configuration",
			"api_key and api_url must be known during the plan. Set them from values that exist before the apply.")
		return
	}
	key := firstSet(cfg.APIKey.ValueString(), os.Getenv(envKeyVar))
	if key == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing API key",
			"Set api_key in the provider block, or set the environment variable "+envKeyVar+".")
		return
	}
	apiURL := firstSet(cfg.APIURL.ValueString(), os.Getenv(envURLVar), client.DefaultURL)
	if strings.HasPrefix(strings.ToLower(apiURL), "http://") {
		resp.Diagnostics.AddAttributeWarning(path.Root("api_url"), "Unencrypted API URL",
			"api_url uses http. The API key and the check UUIDs go over the network with no encryption. Use https.")
	}
	ua := "terraform-provider-healthchecksio/" + p.version + " (+https://github.com/jshvn/terraform-provider-healthchecksio)"
	c, err := client.New(apiURL, key, ua)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}
	c.Log = logRequest
	resp.ResourceData = c
	resp.DataSourceData = c
	resp.ListResourceData = c
	resp.ActionData = c
}

func (p *hcProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newCheckResource}
}

func (p *hcProvider) ListResources(context.Context) []func() list.ListResource {
	return []func() list.ListResource{newCheckList}
}

func (p *hcProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newCheckDataSource,
		newChecksDataSource,
		newChannelsDataSource,
		newBadgesDataSource,
		newPingsDataSource,
		newPingBodyDataSource,
		newFlipsDataSource,
		newStatusDataSource,
	}
}

func (p *hcProvider) Actions(context.Context) []func() action.Action {
	return []func() action.Action{newPauseAction, newResumeAction}
}

// clientFrom gives the client that Configure made. Before Configure, data is nil, and
// clientFrom gives nil with no error. Terraform calls Configure again later.
func clientFrom(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", "The provider data is not a *client.Client. Report this error to the provider maintainers.")
	}
	return c
}

// uuidInLogRE finds a check UUID in a log field. The UUID is the ping credential.
var uuidInLogRE = regexp.MustCompile(uuidPattern)

// logRequest writes one entry of the client to the debug log, with "***" in place of
// each UUID in each field.
func logRequest(ctx context.Context, msg string, fields map[string]any) {
	tflog.Debug(tflog.MaskAllFieldValuesRegexes(ctx, uuidInLogRE), msg, fields)
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
