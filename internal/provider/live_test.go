package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// requireLiveCredentials skips live tests unless TF_ACC and OPENAI_ADMIN_KEY
// are set, since these tests create and archive real projects.
func requireLiveCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run live acceptance tests against the OpenAI Admin API")
	}
	if os.Getenv("OPENAI_ADMIN_KEY") == "" {
		t.Skip("set OPENAI_ADMIN_KEY to run live acceptance tests against the OpenAI Admin API")
	}
}

func liveName() string {
	return "tf-live-" + acctest.RandString(8)
}

func checkLiveProjectsArchived(s *terraform.State) error {
	client, err := newClient(clientConfig{})
	if err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "openai_project" {
			continue
		}
		var found project
		if err := client.do(context.Background(), http.MethodGet, projectID(rs.Primary.ID).path(), nil, &found); err != nil {
			if isNotFound(err) {
				continue
			}
			return err
		}
		if !found.isArchived() {
			return fmt.Errorf("project %s is still %s after destroy", rs.Primary.ID, found.Status)
		}
	}
	return nil
}

func TestLive_Project(t *testing.T) {
	requireLiveCredentials(t)
	name := liveName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLiveProjectsArchived,
		Steps: []resource.TestStep{
			{
				Config: testProjectConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(projectAddress, "id"),
					resource.TestCheckResourceAttr(projectAddress, "name", name),
					resource.TestCheckResourceAttr(projectAddress, "status", "active"),
					resource.TestCheckResourceAttrSet(projectAddress, "created_at"),
				),
			},
			{
				ResourceName:      projectAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config:           testProjectConfig(name + "-renamed"),
				ConfigPlanChecks: expectProjectAction(plancheck.ResourceActionUpdate),
				Check:            resource.TestCheckResourceAttr(projectAddress, "name", name+"-renamed"),
			},
		},
	})
}

func TestLive_ProjectServiceAccount(t *testing.T) {
	requireLiveCredentials(t)
	name := liveName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLiveProjectsArchived,
		Steps: []resource.TestStep{
			{
				Config: testProjectServiceAccountConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(projectServiceAccountAddress, "id"),
					resource.TestCheckResourceAttr(projectServiceAccountAddress, "name", name),
					resource.TestCheckResourceAttrSet(projectServiceAccountAddress, "role"),
					resource.TestCheckResourceAttrSet(projectServiceAccountAddress, "api_key_id"),
					resource.TestCheckResourceAttrSet(projectServiceAccountAddress, "api_key"),
				),
			},
			{
				Config:           testProjectServiceAccountConfig(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:            projectServiceAccountAddress,
				ImportState:             true,
				ImportStateIdFunc:       projectServiceAccountImportID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key", "api_key_id"},
			},
		},
	})
}
