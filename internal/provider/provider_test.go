package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// The acceptance tests need a healthchecks server with a project that has two email
// integrations, a read-write key and a read-only key. `task acc` starts one in a
// container and sets the environment.

// envReadOnlyKeyVar is the read-only key of the test project.
const envReadOnlyKeyVar = "HEALTHCHECKSIO_ACC_READONLY_KEY"

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"healthchecksio": providerserver.NewProtocol6WithError(New("test")()),
}

func preCheck(t *testing.T) {
	for _, v := range []string{envKeyVar, envURLVar, envReadOnlyKeyVar} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests. Use `task acc`.", v)
		}
	}
}

func hasValue(res, name string, check knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(res, tfjsonpath.New(name), check)
}

func emptySet() knownvalue.Check { return knownvalue.SetExact([]knownvalue.Check{}) }

// TestAccCheck changes one check through each kind, and clears each optional value.
// After each step, the test framework makes sure that the next plan is empty.
func TestAccCheck(t *testing.T) {
	name := acctest.RandomWithPrefix("acc")
	res := "healthchecksio_check.t"
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "healthchecksio_channels" "email" { kind = "email" }

resource "healthchecksio_check" "t" {
  name             = %[1]q
  slug             = %[1]q
  tags             = ["acc", "simple"]
  desc             = "first"
  timeout          = 3600
  grace            = 120
  channels         = data.healthchecksio_channels.email.ids
  methods          = "POST"
  manual_resume    = true
  success_kw       = ["OK", "DONE"]
  filter_http_body = true
}`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue(res, "id", knownvalue.StringRegexp(uniqueKeyRE)),
					hasValue(res, "timeout", knownvalue.Int64Exact(3600)),
					hasValue(res, "schedule", knownvalue.Null()),
					hasValue(res, "tz", knownvalue.Null()),
					hasValue(res, "channels", knownvalue.SetSizeExact(2)),
					hasValue(res, "success_kw", knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("DONE"), knownvalue.StringExact("OK")})),
					hasValue(res, "badge_url", knownvalue.StringRegexp(regexp.MustCompile(`/b/2/.+\.svg$`))),
				},
			},
			{
				Config: fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name     = %[1]q
  slug     = %[1]q
  schedule = "*/5 * * * *"
  tz       = "Europe/Riga"
  channels = []
}`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate)}},
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue(res, "timeout", knownvalue.Null()),
					hasValue(res, "schedule", knownvalue.StringExact("*/5 * * * *")),
					hasValue(res, "tz", knownvalue.StringExact("Europe/Riga")),
					hasValue(res, "tags", emptySet()),
					hasValue(res, "desc", knownvalue.StringExact("")),
					hasValue(res, "methods", knownvalue.StringExact("")),
					hasValue(res, "manual_resume", knownvalue.Bool(false)),
					hasValue(res, "success_kw", emptySet()),
					hasValue(res, "filter_http_body", knownvalue.Bool(false)),
					hasValue(res, "channels", emptySet()),
				},
			},
			{
				Config: fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name     = %[1]q
  schedule = "Mon *-*-* 09:00"
}`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue(res, "tz", knownvalue.StringExact("UTC")),
					hasValue(res, "slug", knownvalue.StringExact("")),
				},
			},
			{
				Config: fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name = %[1]q
}`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue(res, "timeout", knownvalue.Int64Exact(defaultTimeout)),
					hasValue(res, "schedule", knownvalue.Null()),
					hasValue(res, "tz", knownvalue.Null()),
				},
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources[res].Primary.Attributes["id"], nil
				},
			},
			{
				ResourceName:    res,
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}

// TestAccDataSources sends one ping with a body to a new check. Then it reads the
// check with each data source.
func TestAccDataSources(t *testing.T) {
	name := acctest.RandomWithPrefix("acc")
	base := fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name = %[1]q
  slug = %[1]q
  tags = [%[1]q]
}`, name)
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: base, Check: sendPing("healthchecksio_check.t", "hello")},
			{
				Config: base + `
data "healthchecksio_check" "by_id" { id = healthchecksio_check.t.id }
data "healthchecksio_check" "by_slug" { slug = healthchecksio_check.t.slug }
data "healthchecksio_checks" "tagged" { tags = [healthchecksio_check.t.name] }
data "healthchecksio_badges" "all" { depends_on = [healthchecksio_check.t] }
data "healthchecksio_pings" "t" { check_id = healthchecksio_check.t.id }
data "healthchecksio_ping_body" "t" {
  check_id = healthchecksio_check.t.id
  n        = data.healthchecksio_pings.t.pings[0].n
}
data "healthchecksio_flips" "t" { check_id = healthchecksio_check.t.id }
data "healthchecksio_status" "s" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue("data.healthchecksio_check.by_id", "status", knownvalue.StringExact("up")),
					hasValue("data.healthchecksio_check.by_id", "n_pings", knownvalue.Int64Exact(1)),
					hasValue("data.healthchecksio_check.by_slug", "name", knownvalue.StringExact(name)),
					hasValue("data.healthchecksio_checks.tagged", "checks", knownvalue.ListSizeExact(1)),
					statecheck.ExpectKnownValue("data.healthchecksio_badges.all", tfjsonpath.New("badges").AtMapKey(name).AtMapKey("svg"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.healthchecksio_pings.t", tfjsonpath.New("pings").AtSliceIndex(0).AtMapKey("has_body"), knownvalue.Bool(true)),
					hasValue("data.healthchecksio_ping_body.t", "body", knownvalue.StringExact("hello")),
					hasValue("data.healthchecksio_ping_body.t", "body_base64", knownvalue.StringExact("aGVsbG8=")),
					hasValue("data.healthchecksio_flips.t", "flips", knownvalue.NotNull()),
					hasValue("data.healthchecksio_status.s", "ok", knownvalue.Bool(true)),
				},
			},
		},
	})
}

// TestAccQuery finds a check with the list resource. Terraform 1.14 and later only.
func TestAccQuery(t *testing.T) {
	name := acctest.RandomWithPrefix("acc")
	base := fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name = %[1]q
  tags = [%[1]q]
}`, name)
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0), skipOpenTofu{}},
		Steps: []resource.TestStep{
			{Config: base},
			{
				Query: true,
				Config: fmt.Sprintf(`
provider "healthchecksio" {}

list "healthchecksio_check" "tagged" {
  provider         = healthchecksio
  include_resource = true
  config {
    tags = [%[1]q]
  }
}`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("healthchecksio_check.tagged", 1),
					querycheck.ExpectIdentity("healthchecksio_check.tagged", map[string]knownvalue.Check{"unique_key": knownvalue.StringRegexp(uniqueKeyRE)}),
				},
			},
		},
	})
}

// TestAccActions pauses a check after its create, then resumes it. Terraform 1.14 and
// later only.
func TestAccActions(t *testing.T) {
	name := acctest.RandomWithPrefix("acc")
	config := func(verb string) string {
		return fmt.Sprintf(`
resource "healthchecksio_check" "t" {
  name = %[1]q
}

action "healthchecksio_%[2]s" "a" {
  config {
    check_id = healthchecksio_check.t.id
  }
}

resource "terraform_data" "%[2]s" {
  input = healthchecksio_check.t.id
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.healthchecksio_%[2]s.a]
    }
  }
}

data "healthchecksio_check" "t" {
  id         = healthchecksio_check.t.id
  depends_on = [terraform_data.%[2]s]
}`, name, verb)
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0), skipOpenTofu{}},
		Steps: []resource.TestStep{
			{
				Config:            config("pause"),
				ConfigStateChecks: []statecheck.StateCheck{hasValue("data.healthchecksio_check.t", "status", knownvalue.StringExact("paused"))},
			},
			{
				Config:            config("resume"),
				ConfigStateChecks: []statecheck.StateCheck{hasValue("data.healthchecksio_check.t", "status", knownvalue.StringExact("new"))},
			},
		},
	})
}

// TestAccCheckDisappears removes the check outside Terraform. The next plan must make
// the check again.
func TestAccCheckDisappears(t *testing.T) {
	res := "healthchecksio_check.t"
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:             fmt.Sprintf(`resource "healthchecksio_check" "t" { name = %q }`, acctest.RandomWithPrefix("acc")),
			Check:              deleteOutside(res),
			ExpectNonEmptyPlan: true,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionCreate)},
			},
		}},
	})
}

// TestAccReadOnlyKey reads a check with a read-only key. The API then sends no uuid, no
// ping_url and no channels, and it refuses the integrations endpoint. The test framework
// serves all provider blocks of a configuration from one provider process, and the last
// configuration applies to all blocks. Thus, the test makes its check with the API, and
// the configuration has only the read-only key.
func TestAccReadOnlyKey(t *testing.T) {
	name := acctest.RandomWithPrefix("acc")
	base := fmt.Sprintf(`
provider "healthchecksio" {
  api_key = %[2]q
}

data "healthchecksio_check" "ro" {
  slug = %[1]q
}
`, name, os.Getenv(envReadOnlyKeyVar))
	ro := "data.healthchecksio_check.ro"
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { makeCheck(t, name) },
				Config: base + fmt.Sprintf(`
data "healthchecksio_checks" "ro" {
  tags = [%[1]q]
}
data "healthchecksio_flips" "ro" {
  check_id = data.healthchecksio_check.ro.id
}
data "healthchecksio_badges" "ro" {}
`, name),
				ConfigStateChecks: []statecheck.StateCheck{
					hasValue(ro, "id", knownvalue.StringRegexp(uniqueKeyRE)),
					hasValue(ro, "uuid", knownvalue.Null()),
					hasValue(ro, "ping_url", knownvalue.Null()),
					hasValue(ro, "channels", knownvalue.Null()),
					hasValue("data.healthchecksio_checks.ro", "checks", knownvalue.ListSizeExact(1)),
					hasValue("data.healthchecksio_flips.ro", "flips", knownvalue.NotNull()),
					statecheck.ExpectKnownValue("data.healthchecksio_badges.ro", tfjsonpath.New("badges").AtMapKey(name), knownvalue.NotNull()),
				},
			},
			{
				Config:      base + `data "healthchecksio_channels" "ro" {}`,
				ExpectError: regexp.MustCompile(`401`),
			},
		},
	})
}

// makeCheck makes a check with the API, with the slug and the tag name. The check goes
// away at the end of the test.
func makeCheck(t *testing.T, name string) {
	t.Helper()
	c, err := testClient()
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CreateCheck(context.Background(), client.CheckSpec{Name: &name, Slug: &name, Tags: &name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.DeleteCheck(context.Background(), got.UUID) })
}

// TestAccValidation makes sure that each value that the server stores in a different
// form is an error. The server changes a legacy time zone name after the apply, and the
// error gives the canonical name. The plan rejects the other values.
func TestAccValidation(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy:             checkDestroyed,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				ExpectError: regexp.MustCompile(`Use\s+Europe/Kyiv`),
				Config: `
resource "healthchecksio_check" "t" {
  name     = "v"
  schedule = "* * * * *"
  tz       = "Europe/Kiev"
}`,
			},
			{
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`lowercase`),
				Config: `
resource "healthchecksio_check" "t" {
  name     = "v"
  channels = ["4EC5A071-2D08-4BAA-898A-EB4EB3CD6941"]
}`,
			},
			{
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`not\s+a\s+UUID`),
				Config:      `data "healthchecksio_check" "t" { id = "31365bce-8da9-4729-8ff3-aaa71d56b712" }`,
			},
		},
	})
}

// testClient gives a read-write client for the test server.
func testClient() (*client.Client, error) {
	return client.New(os.Getenv(envURLVar), os.Getenv(envKeyVar), "acc-test")
}

// checkDestroyed makes sure that the server has no check of the state after the
// destroy.
func checkDestroyed(s *terraform.State) error {
	c, err := testClient()
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "healthchecksio_check" {
			continue
		}
		uuid := rs.Primary.Attributes["uuid"]
		if uuid == "" {
			return fmt.Errorf("check %s: the state has no uuid", rs.Primary.ID)
		}
		_, err := c.GetCheck(context.Background(), uuid)
		switch {
		case err == nil:
			return fmt.Errorf("check %s is on the server after the destroy", rs.Primary.ID)
		case !client.IsNotFound(err):
			return fmt.Errorf("check %s: %w", rs.Primary.ID, err)
		}
	}
	return nil
}

// deleteOutside gives a check function that removes the check res with the API, not
// with Terraform.
func deleteOutside(res string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := testClient()
		if err != nil {
			return err
		}
		return c.DeleteCheck(context.Background(), s.RootModule().Resources[res].Primary.Attributes["uuid"])
	}
}

// sendPing gives a check function that sends a ping with body to the check res. The
// ping URL in the state has the SITE_ROOT of the server, which the test cannot reach.
// Thus, sendPing makes the URL from the API root.
func sendPing(res, body string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		uuid := s.RootModule().Resources[res].Primary.Attributes["uuid"]
		url := strings.TrimSuffix(strings.TrimSuffix(os.Getenv(envURLVar), "/"), "/api/v3") + "/ping/" + uuid
		resp, err := http.Post(url, "text/plain", strings.NewReader(body))
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ping: status %d", resp.StatusCode)
		}
		return nil
	}
}

// The debug log shows "***" in place of each UUID. The UUID is the ping credential. This
// test needs no healthchecks server.
func TestLogRequestMasksUUID(t *testing.T) {
	const uuid = "31365bce-8da9-4729-8ff3-aaa71d56b712"
	var buf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &buf)
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"pings": []}`)) })
	c.Log = logRequest
	if _, err := c.ListPings(ctx, uuid); err != nil {
		t.Fatal(err)
	}
	if log := buf.String(); strings.Contains(log, uuid) || !strings.Contains(log, "checks/***/pings/") {
		t.Fatalf("log = %s", log)
	}
}

// skipOpenTofu skips a test when TF_ACC_TERRAFORM_PATH is OpenTofu. OpenTofu does not
// support list resources and actions, and its version numbers are not Terraform
// versions.
type skipOpenTofu struct{}

func (skipOpenTofu) CheckTerraformVersion(_ context.Context, _ tfversion.CheckTerraformVersionRequest, resp *tfversion.CheckTerraformVersionResponse) {
	if strings.HasSuffix(os.Getenv("TF_ACC_TERRAFORM_PATH"), "tofu") {
		resp.Skip = "OpenTofu does not support this feature"
	}
}
