package django

import (
	"fmt"
	"strings"
)

// DependencyStatus describes whether a runtime dependency is available.
type DependencyStatus struct {
	Name       string `json:"name"`
	Required   bool   `json:"required"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason"`
	Workaround string `json:"workaround,omitempty"`
}

// DependencyReport is a complete dependency check result for a project.
type DependencyReport struct {
	ProjectRoot     string             `json:"project_root"`
	Dependencies    []DependencyStatus `json:"dependencies"`
	MissingRequired int                `json:"missing_required"`
	MissingOptional int                `json:"missing_optional"`
}

// IsHealthy returns true when all required dependencies are available.
func (r DependencyReport) IsHealthy() bool {
	return r.MissingRequired == 0
}

// String returns a human-readable dependency report.
func (r DependencyReport) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Dependency Report (%s)\n", r.ProjectRoot)
	for _, dep := range r.Dependencies {
		state := "ok"
		if !dep.Available {
			state = "missing"
		}
		level := "optional"
		if dep.Required {
			level = "required"
		}
		fmt.Fprintf(&b, "- [%s] %s (%s): %s", state, dep.Name, level, dep.Reason)
		if dep.Workaround != "" {
			fmt.Fprintf(&b, " | workaround: %s", dep.Workaround)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "\nrequired missing: %d\n", r.MissingRequired)
	fmt.Fprintf(&b, "optional missing: %d\n", r.MissingOptional)
	if r.IsHealthy() {
		fmt.Fprintln(&b, "status: PASS")
	} else {
		fmt.Fprintln(&b, "status: FAIL")
	}

	return b.String()
}

func buildDependencyReport(project *Project, hasCmd func(string) bool) DependencyReport {
	if project == nil {
		return DependencyReport{
			ProjectRoot: "<nil>",
			Dependencies: []DependencyStatus{
				{
					Name:       "project context",
					Required:   true,
					Available:  false,
					Reason:     "A discovered Django project is required to evaluate runtime dependencies.",
					Workaround: "Run from a Django project root or pass --project <dir>.",
				},
			},
			MissingRequired: 1,
		}
	}

	report := DependencyReport{
		ProjectRoot: project.RootDir,
	}

	hasPython := hasCmd("python") || hasCmd("python3")
	hasDocker := hasCmd("docker")
	dockerConfigured := project.HasDocker && project.DockerService != ""

	manageBackendAvailable := hasPython || (dockerConfigured && hasDocker)
	report.Dependencies = append(report.Dependencies, DependencyStatus{
		Name:       "manage backend (python or docker)",
		Required:   true,
		Available:  manageBackendAvailable,
		Reason:     "Needed to run Django management commands.",
		Workaround: "Install python/python3, or configure Docker service and install docker.",
	})

	pythonRequired := !dockerConfigured
	report.Dependencies = append(report.Dependencies, DependencyStatus{
		Name:      "python/python3",
		Required:  pythonRequired,
		Available: hasPython,
		Reason:    "Used for direct manage.py execution and local Django operations.",
	})

	dockerRequired := dockerConfigured && !hasPython
	report.Dependencies = append(report.Dependencies, DependencyStatus{
		Name:      "docker",
		Required:  dockerRequired,
		Available: hasDocker,
		Reason:    "Required for Docker-managed Django projects and container actions.",
	})

	report.Dependencies = append(report.Dependencies, DependencyStatus{
		Name:       "git",
		Required:   false,
		Available:  hasCmd("git"),
		Reason:     "Used to enrich snapshot metadata with branch and commit.",
		Workaround: "Snapshots still work without git metadata.",
	})

	report.Dependencies = append(report.Dependencies, DependencyStatus{
		Name:       "lsof",
		Required:   false,
		Available:  hasCmd("lsof"),
		Reason:     "Used for dev-server port detection.",
		Workaround: "Server control still works; status checks may be less accurate.",
	})

	engine := strings.ToLower(project.Database.Engine)
	if strings.Contains(engine, "postgresql") {
		report.Dependencies = append(report.Dependencies,
			DependencyStatus{
				Name:       "pg_dump",
				Required:   false,
				Available:  hasCmd("pg_dump"),
				Reason:     "Native PostgreSQL snapshot export.",
				Workaround: "Fallback uses `manage.py dumpdata` with JSON snapshots.",
			},
			DependencyStatus{
				Name:       "psql",
				Required:   false,
				Available:  hasCmd("psql"),
				Reason:     "Native PostgreSQL snapshot restore for SQL dumps.",
				Workaround: "JSON snapshots restore via `manage.py loaddata`.",
			},
		)
	}

	if strings.Contains(engine, "mysql") {
		report.Dependencies = append(report.Dependencies,
			DependencyStatus{
				Name:       "mysqldump",
				Required:   false,
				Available:  hasCmd("mysqldump"),
				Reason:     "Native MySQL snapshot export.",
				Workaround: "Fallback uses `manage.py dumpdata` with JSON snapshots.",
			},
			DependencyStatus{
				Name:       "mysql",
				Required:   false,
				Available:  hasCmd("mysql"),
				Reason:     "Native MySQL snapshot restore for SQL dumps.",
				Workaround: "JSON snapshots restore via `manage.py loaddata`.",
			},
		)
	}

	for _, dep := range report.Dependencies {
		if dep.Available {
			continue
		}
		if dep.Required {
			report.MissingRequired++
		} else {
			report.MissingOptional++
		}
	}

	return report
}

// BuildDependencyReport returns dependency availability for the given project.
func BuildDependencyReport(project *Project) DependencyReport {
	return buildDependencyReport(project, commandExists)
}
