package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const projectAddress = "openai_project.test"

func testProjectConfig(name string) string {
	return fmt.Sprintf(`
resource "openai_project" "test" {
  name = %q
}
`, name)
}

func expectProjectAction(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(projectAddress, action),
		},
	}
}

func checkFakeProject(fake *fakeAdminAPI, check func(*fakeProject) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id := s.RootModule().Resources[projectAddress].Primary.ID
		fake.mu.Lock()
		defer fake.mu.Unlock()
		project, ok := fake.projects[id]
		if !ok {
			return fmt.Errorf("project %s is missing from the fake", id)
		}
		return check(project)
	}
}

func checkFakeProjectsArchived(fake *fakeAdminAPI) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		for id, project := range fake.projects {
			if project.Status != "archived" || project.ArchivedAt == nil {
				return fmt.Errorf("expected project %s to be archived on destroy, got status %s", id, project.Status)
			}
		}
		return nil
	}
}

func TestAccProject_Basic(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFakeProjectsArchived(fake),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(server) + testProjectConfig("example-project"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectAddress, "id", "proj_0001"),
					resource.TestCheckResourceAttr(projectAddress, "name", "example-project"),
					resource.TestCheckResourceAttr(projectAddress, "created_at", "2024-03-26T16:45:33Z"),
					resource.TestCheckResourceAttr(projectAddress, "status", "active"),
				),
			},
			{
				Config:           testProviderConfig(server) + testProjectConfig("example-project"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				ResourceName:      projectAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config:           testProviderConfig(server) + testProjectConfig("example-project-renamed"),
				ConfigPlanChecks: expectProjectAction(plancheck.ResourceActionUpdate),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(projectAddress, "id", "proj_0001"),
					resource.TestCheckResourceAttr(projectAddress, "name", "example-project-renamed"),
					resource.TestCheckResourceAttr(projectAddress, "status", "active"),
					checkFakeProject(fake, func(project *fakeProject) error {
						if project.Name != "example-project-renamed" {
							return fmt.Errorf("fake project name = %q, want example-project-renamed", project.Name)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestAccProject_GoneOutOfBand(t *testing.T) {
	cases := map[string]func(fake *fakeAdminAPI, id string){
		"archived": func(fake *fakeAdminAPI, id string) { fake.archiveProject(id) },
		"removed":  func(fake *fakeAdminAPI, id string) { fake.removeProject(id) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeAdminAPI()
			server := setupTestServer(t, fake)
			config := testProviderConfig(server) + testProjectConfig("example-project")

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             checkFakeProjectsArchived(fake),
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  resource.TestCheckResourceAttr(projectAddress, "id", "proj_0001"),
					},
					{
						PreConfig: func() {
							fake.mu.Lock()
							defer fake.mu.Unlock()
							change(fake, "proj_0001")
						},
						Config:           config,
						ConfigPlanChecks: expectProjectAction(plancheck.ResourceActionCreate),
						Check:            resource.TestCheckResourceAttr(projectAddress, "id", "proj_0002"),
					},
				},
			})
		})
	}
}

func TestAccProject_ImportArchivedProjectFails(t *testing.T) {
	fake := newFakeAdminAPI()
	server := setupTestServer(t, fake)

	fake.mu.Lock()
	id := fake.seedProject("archived-elsewhere")
	fake.archiveProject(id)
	fake.mu.Unlock()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        testProviderConfig(server) + testProjectConfig("archived-elsewhere"),
				ResourceName:  projectAddress,
				ImportState:   true,
				ImportStateId: id,
				ExpectError:   regexpCannotImportNonExistent,
			},
		},
	})
}
