package django

import "testing"

func hasCommands(commands ...string) func(string) bool {
	set := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		set[command] = struct{}{}
	}
	return func(name string) bool {
		_, ok := set[name]
		return ok
	}
}

func TestBuildDependencyReportMissingRequired(t *testing.T) {
	project := &Project{
		RootDir: "/tmp/project",
		Database: DatabaseInfo{
			Engine: "django.db.backends.sqlite3",
		},
	}

	report := buildDependencyReport(project, hasCommands())
	if report.MissingRequired == 0 {
		t.Fatal("expected missing required dependencies")
	}
	if report.IsHealthy() {
		t.Fatal("report should not be healthy")
	}
}

func TestBuildDependencyReportDockerBackedCore(t *testing.T) {
	project := &Project{
		RootDir:       "/tmp/project",
		HasDocker:     true,
		DockerService: "web",
		Database: DatabaseInfo{
			Engine: "django.db.backends.postgresql",
		},
	}

	report := buildDependencyReport(project, hasCommands("docker", "git"))
	if report.MissingRequired != 0 {
		t.Fatalf("expected no missing required dependencies, got %d", report.MissingRequired)
	}
}

func TestBuildDependencyReportPostgresOptionalTools(t *testing.T) {
	project := &Project{
		RootDir: "/tmp/project",
		Database: DatabaseInfo{
			Engine: "django.db.backends.postgresql",
		},
	}

	report := buildDependencyReport(project, hasCommands("python", "git", "lsof"))
	if report.MissingRequired != 0 {
		t.Fatalf("expected zero missing required, got %d", report.MissingRequired)
	}
	if report.MissingOptional == 0 {
		t.Fatal("expected missing optional postgres tools when pg_dump/psql are absent")
	}
}

func TestBuildDependencyReportNilProject(t *testing.T) {
	report := buildDependencyReport(nil, hasCommands())
	if report.MissingRequired == 0 {
		t.Fatal("expected nil project to report missing required dependency")
	}
	if report.ProjectRoot != "<nil>" {
		t.Fatalf("expected <nil> project root marker, got %q", report.ProjectRoot)
	}
}
