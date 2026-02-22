package django

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Project represents a Django project
type Project struct {
	RootDir           string
	ManagePyPath      string
	SettingsModule    string
	Apps              []App
	Models            []Model
	Migrations        []Migration
	Database          DatabaseInfo
	HasDocker         bool
	DockerService     string // Docker service name for django (e.g., "web", "django")
	DockerComposeFile string // Path to compose file (docker-compose.yml / compose.yaml)
	HasUV             bool
	HasPoetry         bool
	HasPytest         bool
	InstalledApps     []string
	Middleware        []string
	ServerRunning     bool
}

// DiscoverOptions controls the depth of project discovery.
type DiscoverOptions struct {
	// DeepScan runs Django management commands to hydrate settings, models, and migrations.
	// When false, discovery is filesystem-only and significantly faster.
	DeepScan bool
}

// App represents a Django app
type App struct {
	Name   string
	Path   string
	Models []Model
}

// Model represents a Django model
type Model struct {
	Name    string
	App     string
	Fields  int
	HasMeta bool
}

// Migration represents a Django migration
type Migration struct {
	App     string
	Name    string
	Applied bool
}

// DatabaseInfo contains database configuration
type DatabaseInfo struct {
	Engine   string
	Name     string
	Host     string
	Port     string
	User     string
	IsUsable bool
}

type composeServiceSpec struct {
	name string
	body []string
}

// DiscoverProject finds Django project in current or parent directories
func DiscoverProject(startDir string) (*Project, error) {
	return DiscoverProjectWithOptions(startDir, DiscoverOptions{DeepScan: true})
}

// DiscoverProjectWithOptions finds Django project in current or parent directories.
func DiscoverProjectWithOptions(startDir string, opts DiscoverOptions) (*Project, error) {
	dir := startDir
	for {
		managePy := filepath.Join(dir, "manage.py")
		if _, err := os.Stat(managePy); err == nil {
			return buildProjectWithOptions(dir, managePy, opts)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no Django project found (no manage.py)")
		}
		dir = parent
	}
}

// buildProject constructs Project from directory
func buildProject(rootDir, managePy string) (*Project, error) {
	return buildProjectWithOptions(rootDir, managePy, DiscoverOptions{DeepScan: true})
}

func buildProjectWithOptions(rootDir, managePy string, opts DiscoverOptions) (*Project, error) {
	proj := &Project{
		RootDir:      rootDir,
		ManagePyPath: managePy,
	}

	// Check for Docker first
	// Find compose file (support docker-compose.yml, docker-compose.yaml, compose.yaml)
	proj.DockerComposeFile = findComposeFile(rootDir)
	if proj.DockerComposeFile != "" || fileExists(filepath.Join(rootDir, "Dockerfile")) {
		proj.HasDocker = true
	}

	// Discover Docker service if compose file found
	if proj.DockerComposeFile != "" {
		proj.DockerService = findDjangoService(proj.DockerComposeFile)
	}

	// Discover apps
	apps, err := discoverApps(rootDir)
	if err == nil {
		proj.Apps = apps
	}

	// Check for other tools
	proj.HasUV = commandExists("uv")
	proj.HasPoetry = fileExists(filepath.Join(rootDir, "poetry.lock"))
	proj.HasPytest = fileExists(filepath.Join(rootDir, "pytest.ini")) ||
		fileExists(filepath.Join(rootDir, "pyproject.toml"))

	if opts.DeepScan {
		// Discover settings and database info.
		proj.DiscoverSettings()
		proj.DiscoverModels()
		proj.DiscoverMigrations()
	} else {
		// Fast path for UI startup: keep metadata lightweight and defer deep discovery.
		if module := os.Getenv("DJANGO_SETTINGS_MODULE"); module != "" {
			proj.SettingsModule = module
		}
	}

	return proj, nil
}

// discoverApps finds Django apps in project
func discoverApps(rootDir string) ([]App, error) {
	var apps []App

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip common non-app directories
		base := filepath.Base(path)
		if base == "venv" || base == "node_modules" || base == ".git" ||
			base == "__pycache__" || base == "migrations" {
			return filepath.SkipDir
		}

		// Check if directory has apps.py or models.py (marks it as an app)
		if info.IsDir() {
			appsFile := filepath.Join(path, "apps.py")
			modelsFile := filepath.Join(path, "models.py")
			if fileExists(appsFile) || fileExists(modelsFile) {
				relPath, _ := filepath.Rel(rootDir, path)
				appName := strings.ReplaceAll(relPath, string(os.PathSeparator), ".")
				apps = append(apps, App{
					Name: appName,
					Path: relPath,
				})
			}
		}

		return nil
	})

	return apps, err
}

// fileExists checks if file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// commandExists checks if command is available
func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

var quotedStringPattern = regexp.MustCompile(`['"]([^'"]+)['"]`)

func pythonBinary() string {
	if py, err := exec.LookPath("python"); err == nil {
		return py
	}
	if py3, err := exec.LookPath("python3"); err == nil {
		return py3
	}
	return "python"
}

func extractLikelySettingsModule(line string) string {
	matches := quotedStringPattern.FindAllStringSubmatch(line, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(matches[i][1])
		if candidate == "" || candidate == "DJANGO_SETTINGS_MODULE" {
			continue
		}
		if strings.Contains(candidate, ".") {
			return candidate
		}
	}
	return ""
}

// RunCommand executes Django management command
func (p *Project) RunCommand(args ...string) (string, error) {
	if p.HasDocker && p.DockerService != "" && isDockerAvailable() {
		output, err := p.runDockerCommandForService(p.DockerService, args...)
		if err == nil || !shouldRetryWithAlternateDockerService(output, err) {
			return output, err
		}

		for _, service := range findDjangoServiceCandidates(p.DockerComposeFile) {
			service = strings.TrimSpace(service)
			if service == "" || service == p.DockerService {
				continue
			}

			nextOutput, nextErr := p.runDockerCommandForService(service, args...)
			if nextErr == nil {
				// Promote a working service for subsequent commands.
				p.DockerService = service
				return nextOutput, nil
			}
			if !shouldRetryWithAlternateDockerService(nextOutput, nextErr) {
				return nextOutput, nextErr
			}
			output, err = nextOutput, nextErr
		}

		return output, err
	}

	cmd := exec.Command(pythonBinary(), append([]string{p.ManagePyPath}, args...)...)
	cmd.Dir = p.RootDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func shouldRetryWithAlternateDockerService(output string, err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(strings.TrimSpace(output + "\n" + err.Error()))
	if text == "" {
		return false
	}

	// Retry only when docker exec failed to target a usable service.
	hints := []string{
		"is not running",
		"no such service",
		"service not found",
		"container is not running",
		"cannot exec in a stopped state",
		"oci runtime exec failed",
		"executable file not found in $path",
	}
	for _, hint := range hints {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

func (p *Project) runDockerCommandForService(service string, args ...string) (string, error) {
	cmd := p.buildDockerCommandForService(service, args...)
	cmd.Dir = p.RootDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// buildDockerCommand creates a Docker-based manage.py command
func (p *Project) buildDockerCommand(args ...string) *exec.Cmd {
	return p.buildDockerCommandForService(p.DockerService, args...)
}

func (p *Project) buildDockerCommandForService(service string, args ...string) *exec.Cmd {
	service = strings.TrimSpace(service)
	if service == "" {
		service = p.DockerService
	}

	composeArgs := []string{"compose"}
	if p.DockerComposeFile != "" {
		composeArgs = append(composeArgs, "-f", p.DockerComposeFile)
	}
	composeArgs = append(composeArgs, "exec", "-T", service, "python", "manage.py")
	composeArgs = append(composeArgs, args...)

	// Try docker (v2) first, fallback to docker-compose
	if dockerPath, err := exec.LookPath("docker"); err == nil {
		return exec.Command(dockerPath, composeArgs...)
	}
	if dcPath, err := exec.LookPath("docker-compose"); err == nil {
		// Remove "compose" from args for docker-compose v1
		dcArgs := composeArgs[1:]
		return exec.Command(dcPath, dcArgs...)
	}

	// Fallback to direct python if docker tools not available
	return exec.Command(pythonBinary(), append([]string{p.ManagePyPath}, args...)...)
}

// GetMigrations returns list of migrations for an app
func (p *Project) GetMigrations(appName string) ([]string, error) {
	output, err := p.RunCommand("showmigrations", appName, "--list")
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			migrations = append(migrations, line)
		}
	}
	return migrations, nil
}

// DiscoverSettings reads Django settings information
func (p *Project) DiscoverSettings() {
	// Try to get DJANGO_SETTINGS_MODULE
	if module := os.Getenv("DJANGO_SETTINGS_MODULE"); module != "" {
		p.SettingsModule = module
	} else {
		p.SettingsModule = findSettingsModule(p.RootDir)
	}

	// Try Django shell first (most accurate)
	cmd := `import json; from django.conf import settings; print(json.dumps({"apps": list(settings.INSTALLED_APPS), "middleware": list(getattr(settings, "MIDDLEWARE", [])), "debug": getattr(settings, "DEBUG", False), "databases": {k: {"ENGINE": v.get("ENGINE", ""), "NAME": v.get("NAME", ""), "HOST": v.get("HOST", ""), "PORT": str(v.get("PORT", "")), "USER": v.get("USER", "")} for k, v in settings.DATABASES.items()}}))`
	output, err := p.RunCommand("shell", "-c", cmd)
	if err == nil {
		p.parseSettingsJSON(output)
	}

	// Fallback: parse settings files directly if Django shell failed
	if len(p.InstalledApps) == 0 || !p.Database.IsUsable {
		parseSettingsFiles(p)
	}

	// If we have Docker, try to resolve database environment variables from compose
	if p.HasDocker && p.DockerComposeFile != "" {
		resolveDockerDatabaseEnv(p)
	}
}

// parseSettingsJSON extracts settings from Django shell JSON output
func (p *Project) parseSettingsJSON(output string) {
	var data map[string]interface{}

	// Find JSON in output (skip startup messages)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &data) != nil {
				return
			}
			break
		}
	}

	// Extract apps
	if apps, ok := data["apps"].([]interface{}); ok {
		for _, app := range apps {
			if appStr, ok := app.(string); ok {
				p.InstalledApps = append(p.InstalledApps, appStr)
			}
		}
	}

	// Extract middleware
	if middleware, ok := data["middleware"].([]interface{}); ok {
		for _, mw := range middleware {
			if mwStr, ok := mw.(string); ok {
				p.Middleware = append(p.Middleware, mwStr)
			}
		}
	}

	// Extract database config
	if databases, ok := data["databases"].(map[string]interface{}); ok {
		if defaultDB, ok := databases["default"].(map[string]interface{}); ok {
			if engine, ok := defaultDB["ENGINE"].(string); ok {
				p.Database.Engine = engine
			}
			if name, ok := defaultDB["NAME"].(string); ok {
				p.Database.Name = name
			}
			if host, ok := defaultDB["HOST"].(string); ok {
				p.Database.Host = host
			}
			if port, ok := defaultDB["PORT"].(string); ok {
				p.Database.Port = port
			}
			if user, ok := defaultDB["USER"].(string); ok {
				p.Database.User = user
			}
			p.Database.IsUsable = true
		}
	}
}

// DiscoverModels finds Django models in the project
func (p *Project) DiscoverModels() {
	// Use Django's introspection to get actual models
	cmd := `import json; from django.apps import apps; models_data = [{"app": m._meta.app_label, "name": m.__name__, "fields": len(m._meta.fields)} for m in apps.get_models()]; print(json.dumps(models_data))`
	output, err := p.RunCommand("shell", "-c", cmd)

	if err == nil {
		var modelsData []map[string]interface{}
		// Clean output (might have startup messages)
		lines := strings.Split(strings.TrimSpace(output), "\n")
		jsonLine := ""
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "[") {
				jsonLine = strings.TrimSpace(line)
				break
			}
		}

		if jsonLine != "" && json.Unmarshal([]byte(jsonLine), &modelsData) == nil {
			// Clear existing models
			p.Models = []Model{}
			for i := range p.Apps {
				p.Apps[i].Models = []Model{}
			}

			// Populate with discovered models
			for _, modelData := range modelsData {
				model := Model{
					Name:   modelData["name"].(string),
					App:    modelData["app"].(string),
					Fields: int(modelData["fields"].(float64)),
				}
				p.Models = append(p.Models, model)

				// Add to corresponding app
				for i := range p.Apps {
					if p.Apps[i].Name == model.App {
						p.Apps[i].Models = append(p.Apps[i].Models, model)
						break
					}
				}
			}
			return
		}
	}

	// Fallback to file parsing if Django introspection fails
	for i := range p.Apps {
		modelsFile := filepath.Join(p.RootDir, p.Apps[i].Path, "models.py")
		if fileExists(modelsFile) {
			models := parseModelsFile(modelsFile, p.Apps[i].Name)
			p.Apps[i].Models = models
			p.Models = append(p.Models, models...)
		}
	}
}

// parseModelsFile extracts model information from models.py
func parseModelsFile(path string, appName string) []Model {
	var models []Model
	content, err := os.ReadFile(path)
	if err != nil {
		return models
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for class definitions that inherit from models.Model
		if strings.HasPrefix(trimmed, "class ") && strings.Contains(trimmed, "models.Model") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				modelName := strings.TrimSuffix(parts[1], "(models.Model):")
				modelName = strings.TrimSuffix(modelName, "(models.Model)")
				modelName = strings.TrimSuffix(modelName, ":")
				models = append(models, Model{
					Name: modelName,
					App:  appName,
				})
			}
		}
	}
	return models
}

// DiscoverMigrations finds all migrations in the project
func (p *Project) DiscoverMigrations() {
	output, err := p.RunCommand("showmigrations", "--list")
	if err == nil {
		// Try Django command first
		var currentApp string
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// App name (no leading bracket)
			if !strings.HasPrefix(line, "[") {
				currentApp = line
				continue
			}

			// Migration line
			if strings.HasPrefix(line, "[") && currentApp != "" {
				applied := strings.HasPrefix(line, "[X]") || strings.HasPrefix(line, "[x]")
				name := strings.TrimPrefix(line, "[X] ")
				name = strings.TrimPrefix(name, "[x] ")
				name = strings.TrimPrefix(name, "[ ] ")
				name = strings.TrimSpace(name)

				p.Migrations = append(p.Migrations, Migration{
					App:     currentApp,
					Name:    name,
					Applied: applied,
				})
			}
		}
		return
	}

	// Fallback: scan migration files directly
	for _, app := range p.Apps {
		migrationsDir := filepath.Join(p.RootDir, app.Path, "migrations")
		if !fileExists(migrationsDir) {
			continue
		}

		files, err := os.ReadDir(migrationsDir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".py") || file.Name() == "__init__.py" {
				continue
			}

			// Extract migration name (remove .py extension)
			migrationName := strings.TrimSuffix(file.Name(), ".py")
			p.Migrations = append(p.Migrations, Migration{
				App:     app.Name,
				Name:    migrationName,
				Applied: false, // Can't determine without Django
			})
		}
	}
}

// GetURLPatterns returns URL patterns from Django
func (p *Project) GetURLPatterns() ([]string, error) {
	cmd := `from django.urls import get_resolver; from django.urls.resolvers import URLPattern, URLResolver; def show_urls(patterns, prefix=''):
    for pattern in patterns:
        if isinstance(pattern, URLPattern):
            print(f"{prefix}{pattern.pattern}")
        elif isinstance(pattern, URLResolver):
            show_urls(pattern.url_patterns, prefix + str(pattern.pattern))
show_urls(get_resolver().url_patterns)`
	output, err := p.RunCommand("shell", "-c", strings.ReplaceAll(cmd, "\n", "; "))
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

// IsServerRunning checks if dev server is running
func (p *Project) IsServerRunning() bool {
	// Check if port 8000 is in use (basic check)
	cmd := exec.Command("lsof", "-i", ":8000", "-sTCP:LISTEN")
	err := cmd.Run()
	return err == nil
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func stripYAMLComment(line string) string {
	// Keep this conservative; compose service names do not need quote-aware parsing.
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func parseComposeServices(composeContent string) []composeServiceSpec {
	lines := strings.Split(composeContent, "\n")
	inServices := false
	servicesIndent := -1

	var services []composeServiceSpec
	current := -1
	for _, raw := range lines {
		line := stripYAMLComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}

		indent := leadingSpaces(line)
		trimmed := strings.TrimSpace(line)

		if !inServices {
			if trimmed == "services:" {
				inServices = true
				servicesIndent = indent
			}
			continue
		}

		if indent <= servicesIndent {
			break
		}

		if indent == servicesIndent+2 && strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))
			if name == "" {
				continue
			}
			services = append(services, composeServiceSpec{name: name})
			current = len(services) - 1
			continue
		}

		if current >= 0 && indent > servicesIndent+2 {
			services[current].body = append(services[current].body, trimmed)
		}
	}

	return services
}

func scoreDjangoService(spec composeServiceSpec) int {
	name := strings.ToLower(strings.TrimSpace(spec.name))
	body := strings.ToLower(strings.Join(spec.body, "\n"))
	score := 0

	switch name {
	case "web", "django", "app", "backend", "django-app":
		score += 250
	}
	if strings.Contains(name, "django") {
		score += 120
	}
	if strings.Contains(name, "web") {
		score += 80
	}
	if strings.Contains(name, "app") {
		score += 60
	}
	if strings.Contains(name, "api") {
		score += 40
	}
	if strings.Contains(name, "admin") {
		score += 30
	}

	if strings.Contains(name, "worker") || strings.Contains(name, "celery") || strings.Contains(name, "rq") {
		score -= 180
	}
	if strings.Contains(name, "redis") || strings.Contains(name, "postgres") ||
		strings.Contains(name, "mysql") || strings.Contains(name, "db") ||
		strings.Contains(name, "proxy") || strings.Contains(name, "webpack") ||
		strings.Contains(name, "nginx") || strings.Contains(name, "rabbit") {
		score -= 260
	}

	if strings.Contains(body, "manage.py runserver") {
		score += 260
	}
	if strings.Contains(body, "python manage.py") || strings.Contains(body, "./manage.py") {
		score += 170
	}
	if strings.Contains(body, "gunicorn") || strings.Contains(body, "uvicorn") || strings.Contains(body, "daphne") {
		score += 110
	}
	if strings.Contains(body, "django_settings_module") {
		score += 50
	}
	if strings.Contains(body, "rqworker") || strings.Contains(body, "celery worker") {
		score -= 180
	}
	if strings.Contains(body, "redis-server") || strings.Contains(body, "postgres:") {
		score -= 120
	}

	return score
}

func findDjangoServiceCandidates(composePath string) []string {
	composePath = strings.TrimSpace(composePath)
	if composePath == "" {
		return nil
	}

	content, err := os.ReadFile(composePath)
	if err != nil {
		return nil
	}

	specs := parseComposeServices(string(content))
	if len(specs) == 0 {
		return nil
	}

	type rankedService struct {
		name  string
		score int
	}
	ranked := make([]rankedService, 0, len(specs))
	for _, spec := range specs {
		ranked = append(ranked, rankedService{
			name:  spec.name,
			score: scoreDjangoService(spec),
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].name < ranked[j].name
		}
		return ranked[i].score > ranked[j].score
	})

	names := make([]string, 0, len(ranked))
	seen := make(map[string]struct{}, len(ranked))
	for _, candidate := range ranked {
		name := strings.TrimSpace(candidate.name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// findDjangoService finds the Django service name in docker-compose.yml
func findDjangoService(composePath string) string {
	candidates := findDjangoServiceCandidates(composePath)
	if len(candidates) == 0 {
		return "web"
	}
	return candidates[0]
}

// findSettingsModule recursively searches for Django settings module
func findSettingsModule(rootDir string) string {
	var settingsModule string

	// First try to read from compose.yaml for DJANGO_SETTINGS_MODULE
	composeFile := findComposeFile(rootDir)
	if composeFile != "" {
		content, err := os.ReadFile(composeFile)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "DJANGO_SETTINGS_MODULE") {
					trimmed := strings.TrimSpace(strings.Split(line, "#")[0])
					trimmed = strings.TrimPrefix(trimmed, "- ")

					var module string
					if idx := strings.Index(trimmed, "="); idx >= 0 {
						module = strings.TrimSpace(trimmed[idx+1:])
					} else if idx := strings.Index(trimmed, ":"); idx >= 0 {
						module = strings.TrimSpace(trimmed[idx+1:])
					}

					module = strings.Trim(module, "\"'")
					if module != "" {
						return module
					}
				}
			}
		}
	}

	// Try to read from manage.py or wsgi.py
	managePy := filepath.Join(rootDir, "manage.py")
	if fileExists(managePy) {
		content, err := os.ReadFile(managePy)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "DJANGO_SETTINGS_MODULE") {
					if module := extractLikelySettingsModule(line); module != "" {
						return module
					}
				}
			}
		}
	}

	// Check wsgi.py files
	wsgiFiles, _ := filepath.Glob(filepath.Join(rootDir, "*/wsgi.py"))
	for _, wsgiFile := range wsgiFiles {
		content, err := os.ReadFile(wsgiFile)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "DJANGO_SETTINGS_MODULE") {
					if module := extractLikelySettingsModule(line); module != "" {
						return module
					}
				}
			}
		}
	}

	// Search for settings.py or settings/ directory
	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip common non-settings directories
		base := filepath.Base(path)
		if base == "venv" || base == "node_modules" || base == ".git" ||
			base == "__pycache__" || base == "migrations" || base == "static" ||
			base == "media" || base == "staticfiles" {
			return filepath.SkipDir
		}

		// Look for settings.py
		if !info.IsDir() && base == "settings.py" {
			relPath, _ := filepath.Rel(rootDir, filepath.Dir(path))
			settingsModule = strings.ReplaceAll(relPath, string(os.PathSeparator), ".") + ".settings"
			return filepath.SkipAll
		}

		// Look for settings/ directory with __init__.py
		if info.IsDir() && base == "settings" {
			initPath := filepath.Join(path, "__init__.py")
			if fileExists(initPath) {
				relPath, _ := filepath.Rel(rootDir, path)
				settingsModule = strings.ReplaceAll(relPath, string(os.PathSeparator), ".")
				return filepath.SkipAll
			}
		}

		return nil
	})

	// Fallback: try common patterns
	if settingsModule == "" {
		// Look for */settings.py pattern
		matches, _ := filepath.Glob(filepath.Join(rootDir, "*", "settings.py"))
		if len(matches) > 0 {
			dir := filepath.Base(filepath.Dir(matches[0]))
			settingsModule = dir + ".settings"
		}
	}

	return settingsModule
} // findComposeFile looks for common compose filenames and returns the full path if present
func findComposeFile(rootDir string) string {
	candidates := []string{
		filepath.Join(rootDir, "docker-compose.yml"),
		filepath.Join(rootDir, "docker-compose.yaml"),
		filepath.Join(rootDir, "compose.yaml"),
		filepath.Join(rootDir, "compose.yml"),
	}

	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// extractEnvFromCompose reads environment variables from compose file and referenced .env files
func extractEnvFromCompose(composeFile string) map[string]string {
	env := make(map[string]string)

	content, err := os.ReadFile(composeFile)
	if err != nil {
		return env
	}

	rootDir := filepath.Dir(composeFile)
	lines := strings.Split(string(content), "\n")
	inEnvironment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for env_file references
		if strings.HasPrefix(trimmed, "env_file:") {
			// Next lines contain file references
			continue
		}
		if strings.HasPrefix(trimmed, "- ./") || strings.HasPrefix(trimmed, "- ") {
			// This might be an env file reference
			envFilePath := strings.TrimPrefix(trimmed, "- ")
			envFilePath = strings.Trim(envFilePath, "\"'")
			if strings.HasSuffix(envFilePath, ".env") || strings.Contains(envFilePath, ".env") {
				fullPath := filepath.Join(rootDir, envFilePath)
				if envContent, err := os.ReadFile(fullPath); err == nil {
					for _, envLine := range strings.Split(string(envContent), "\n") {
						envLine = strings.TrimSpace(envLine)
						if envLine == "" || strings.HasPrefix(envLine, "#") {
							continue
						}
						if idx := strings.Index(envLine, "="); idx > 0 {
							key := strings.TrimSpace(envLine[:idx])
							value := strings.TrimSpace(envLine[idx+1:])
							env[key] = value
						}
					}
				}
			}
		}

		// Check for environment section
		if strings.HasPrefix(trimmed, "environment:") {
			inEnvironment = true
			continue
		}

		// Parse environment variables
		if inEnvironment {
			// End of environment section
			if trimmed != "" && !strings.HasPrefix(trimmed, "-") && !strings.Contains(trimmed, "=") && strings.HasSuffix(trimmed, ":") {
				inEnvironment = false
				continue
			}

			// Parse environment variable line
			if strings.HasPrefix(trimmed, "- ") {
				envVar := strings.TrimPrefix(trimmed, "- ")
				if idx := strings.Index(envVar, "="); idx > 0 {
					key := strings.TrimSpace(envVar[:idx])
					value := strings.TrimSpace(envVar[idx+1:])
					env[key] = value
				}
			}
		}
	}

	return env
}

// resolveDockerDatabaseEnv resolves database configuration from Docker environment
func resolveDockerDatabaseEnv(p *Project) {
	if p.DockerComposeFile == "" {
		return
	}

	env := extractEnvFromCompose(p.DockerComposeFile)

	// Try to resolve database configuration from environment variables
	if dbHost, ok := env["DB_HOST"]; ok && dbHost != "" {
		p.Database.Host = dbHost
		p.Database.IsUsable = true
	}
	if dbName, ok := env["DB_NAME"]; ok && dbName != "" {
		p.Database.Name = dbName
	}
	if dbPort, ok := env["DB_PORT"]; ok && dbPort != "" {
		p.Database.Port = dbPort
	} else if p.Database.Host != "" && p.Database.Port == "" {
		// Default PostgreSQL port if host is set but port isn't
		p.Database.Port = "5432"
	}
	if dbUser, ok := env["DB_USER"]; ok && dbUser != "" {
		p.Database.User = dbUser
	}

	// If DB_HOST is set, assume PostgreSQL
	if p.Database.Host != "" && p.Database.Engine == "" {
		p.Database.Engine = "django.db.backends.postgresql"
	}

	// Handle sqlite fallback case - check for DB_LOCAL
	if p.Database.Host == "" && p.Database.Name == "" {
		if dbLocal, ok := env["DB_LOCAL"]; ok && dbLocal != "" {
			p.Database.Engine = "django.db.backends.sqlite3"
			p.Database.Name = dbLocal
			p.Database.IsUsable = true
		}
	}
}

// isDockerAvailable checks if Docker daemon is running
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	err := cmd.Run()
	return err == nil
}

// parseSettingsFiles tries to parse Django settings files directly
func parseSettingsFiles(p *Project) {
	if p.SettingsModule == "" {
		return
	}

	// Convert module path to file path
	moduleParts := strings.Split(p.SettingsModule, ".")
	settingsPath := filepath.Join(p.RootDir, filepath.Join(moduleParts...))

	// Try both .py file and __init__.py in directory, plus common patterns
	settingsCandidates := []string{
		settingsPath + ".py",
		filepath.Join(settingsPath, "__init__.py"),
		filepath.Join(settingsPath, "prod.py"),
		filepath.Join(settingsPath, "base.py"),
		filepath.Join(settingsPath, "dev.py"),
		filepath.Join(settingsPath, "local.py"),
		filepath.Join(settingsPath, "production.py"),
		filepath.Join(settingsPath, "development.py"),
	}

	// Also try common/settings/prod.py pattern
	for i := range moduleParts {
		if moduleParts[i] == "settings" && i > 0 {
			commonPath := filepath.Join(p.RootDir, filepath.Join(moduleParts[:i]...), "common", "settings")
			settingsCandidates = append(settingsCandidates,
				filepath.Join(commonPath, "prod.py"),
				filepath.Join(commonPath, "base.py"),
				filepath.Join(commonPath, "__init__.py"),
			)
		}
	}

	parsedFiles := make(map[string]bool)
	for _, candidate := range settingsCandidates {
		parseSettingsFileRecursive(candidate, p, parsedFiles)

		// If we found data, we're done
		if len(p.InstalledApps) > 0 && p.Database.IsUsable {
			return
		}
	}
}

// parseSettingsFileRecursive parses a settings file and follows imports
func parseSettingsFileRecursive(filePath string, p *Project, parsedFiles map[string]bool) {
	if !fileExists(filePath) || parsedFiles[filePath] {
		return
	}

	parsedFiles[filePath] = true

	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")

	// First, check for imports and follow them
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "from ") && strings.Contains(trimmed, "import *") {
			// Extract the module path: from sites.common.settings.prod import *
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				modulePath := parts[1]
				// Convert to file path
				moduleFile := filepath.Join(p.RootDir, strings.ReplaceAll(modulePath, ".", string(os.PathSeparator))) + ".py"
				parseSettingsFileRecursive(moduleFile, p, parsedFiles)
			}
		}
	}

	// Then parse this file's content
	parseSettingsContent(lines, p)
}

// parseSettingsContent extracts configuration from settings file lines
func parseSettingsContent(lines []string, p *Project) {
	inDatabases := false
	inInstalledApps := false
	inMiddleware := false
	bracketDepth := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Track DATABASES dictionary
		if strings.HasPrefix(trimmed, "DATABASES") && strings.Contains(trimmed, "=") {
			inDatabases = true
			bracketDepth = 0
		}
		if inDatabases {
			bracketDepth += strings.Count(line, "{") - strings.Count(line, "}")

			// Extract ENGINE
			if strings.Contains(trimmed, "'ENGINE'") || strings.Contains(trimmed, "\"ENGINE\"") {
				if idx := strings.Index(trimmed, ":"); idx > 0 {
					value := strings.TrimSpace(trimmed[idx+1:])
					value = strings.Trim(value, "',\",")
					p.Database.Engine = value
					p.Database.IsUsable = true
				}
			}
			// Extract NAME
			if strings.Contains(trimmed, "'NAME'") || strings.Contains(trimmed, "\"NAME\"") {
				if idx := strings.Index(trimmed, ":"); idx > 0 {
					value := strings.TrimSpace(trimmed[idx+1:])
					value = strings.Trim(value, "',\",")
					p.Database.Name = value
				}
			}
			// Extract HOST
			if strings.Contains(trimmed, "'HOST'") || strings.Contains(trimmed, "\"HOST\"") {
				if idx := strings.Index(trimmed, ":"); idx > 0 {
					value := strings.TrimSpace(trimmed[idx+1:])
					value = strings.Trim(value, "',\",")
					p.Database.Host = value
				}
			}
			// Extract PORT
			if strings.Contains(trimmed, "'PORT'") || strings.Contains(trimmed, "\"PORT\"") {
				if idx := strings.Index(trimmed, ":"); idx > 0 {
					value := strings.TrimSpace(trimmed[idx+1:])
					value = strings.Trim(value, "',\",")
					p.Database.Port = value
				}
			}

			if bracketDepth == 0 && i > 0 {
				inDatabases = false
			}
		}

		// Track INSTALLED_APPS list
		if strings.HasPrefix(trimmed, "INSTALLED_APPS") && strings.Contains(trimmed, "=") {
			inInstalledApps = true
			bracketDepth = 0
		}
		if inInstalledApps {
			bracketDepth += strings.Count(line, "[") + strings.Count(line, "(")
			bracketDepth -= strings.Count(line, "]") + strings.Count(line, ")")

			// Extract app names
			if strings.Contains(trimmed, "'") || strings.Contains(trimmed, "\"") {
				// Extract quoted strings
				parts := strings.FieldsFunc(trimmed, func(r rune) bool {
					return r == '\'' || r == '"'
				})
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" && !strings.Contains(part, ",") && !strings.Contains(part, "[") && !strings.Contains(part, "]") {
						// Looks like an app name
						if strings.Contains(part, ".") || strings.Contains(part, "_") {
							p.InstalledApps = append(p.InstalledApps, part)
						}
					}
				}
			}

			if bracketDepth == 0 && i > 0 {
				inInstalledApps = false
			}
		}

		// Track MIDDLEWARE list
		if strings.HasPrefix(trimmed, "MIDDLEWARE") && strings.Contains(trimmed, "=") {
			inMiddleware = true
			bracketDepth = 0
		}
		if inMiddleware {
			bracketDepth += strings.Count(line, "[") + strings.Count(line, "(")
			bracketDepth -= strings.Count(line, "]") + strings.Count(line, ")")

			// Extract middleware names
			if strings.Contains(trimmed, "'") || strings.Contains(trimmed, "\"") {
				parts := strings.FieldsFunc(trimmed, func(r rune) bool {
					return r == '\'' || r == '"'
				})
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" && !strings.Contains(part, ",") && !strings.Contains(part, "[") && !strings.Contains(part, "]") {
						if strings.Contains(part, ".") {
							p.Middleware = append(p.Middleware, part)
						}
					}
				}
			}

			if bracketDepth == 0 && i > 0 {
				inMiddleware = false
			}
		}
	}
}
