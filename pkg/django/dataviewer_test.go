package django

import (
	"testing"
)

// MockProject implements RunCommand for testing
type MockProject struct {
	Project
	mockResponse string
	mockError    error
}

func (m *MockProject) RunCommand(args ...string) (string, error) {
	if m.mockError != nil {
		return "", m.mockError
	}
	return m.mockResponse, nil
}

// TestDataViewer tests the data viewer functionality
func TestDataViewer(t *testing.T) {
	// Create mock project that simulates Django commands
	mockProj := &MockProject{}
	dv := NewDataViewer(mockProj)

	t.Run("QueryModel", func(t *testing.T) {
		// Set mock response
		mockProj.mockResponse = `{
			"records": [
				{"pk": 1, "model": "blog.Post", "fields": {"title": "Test Post", "content": "Content"}},
				{"pk": 2, "model": "blog.Post", "fields": {"title": "Another Post", "content": "More content"}}
			],
			"total": 2,
			"page": 1,
			"page_size": 50,
			"has_next": false,
			"has_prev": false
		}`

		result, err := dv.QueryModel("blog", "Post", nil, 1, 50)
		if err != nil {
			t.Fatalf("QueryModel failed: %v", err)
		}

		if len(result.Records) != 2 {
			t.Errorf("Expected 2 records, got %d", len(result.Records))
		}

		if result.Total != 2 {
			t.Errorf("Expected total 2, got %d", result.Total)
		}

		if result.Page != 1 {
			t.Errorf("Expected page 1, got %d", result.Page)
		}
	})

	t.Run("QueryModelWithPagination", func(t *testing.T) {
		mockProj.mockResponse = `{
			"records": [],
			"total": 100,
			"page": 2,
			"page_size": 20,
			"has_next": true,
			"has_prev": true
		}`

		result, err := dv.QueryModel("blog", "Post", nil, 2, 20)
		if err != nil {
			t.Fatalf("QueryModel failed: %v", err)
		}

		if !result.HasNext {
			t.Error("Expected HasNext to be true")
		}

		if !result.HasPrev {
			t.Error("Expected HasPrev to be true")
		}
	})

	t.Run("QueryModelWithFilters", func(t *testing.T) {
		mockProj.mockResponse = `{
			"records": [{"pk": 1, "model": "blog.Post", "fields": {"title": "Filtered"}}],
			"total": 1,
			"page": 1,
			"page_size": 50,
			"has_next": false,
			"has_prev": false
		}`

		filters := map[string]string{
			"is_published": "true",
			"author":       "admin",
		}

		result, err := dv.QueryModel("blog", "Post", filters, 1, 50)
		if err != nil {
			t.Fatalf("QueryModel with filters failed: %v", err)
		}

		if len(result.Records) != 1 {
			t.Errorf("Expected 1 filtered record, got %d", len(result.Records))
		}
	})

	t.Run("GetRecord", func(t *testing.T) {
		mockProj.mockResponse = `{
			"pk": 1,
			"model": "blog.Post",
			"fields": {
				"title": "Test Post",
				"content": "Test Content",
				"author": "admin"
			}
		}`

		record, err := dv.GetRecord("blog", "Post", 1)
		if err != nil {
			t.Fatalf("GetRecord failed: %v", err)
		}

		if record.PK != float64(1) {
			t.Errorf("Expected PK 1, got %v", record.PK)
		}

		if record.Fields["title"] != "Test Post" {
			t.Errorf("Expected title 'Test Post', got '%v'", record.Fields["title"])
		}
	})

	t.Run("CreateRecord", func(t *testing.T) {
		mockProj.mockResponse = `{"pk": 3, "success": true}`

		fields := map[string]interface{}{
			"title":   "New Post",
			"content": "New Content",
			"author":  "user",
		}

		pk, err := dv.CreateRecord("blog", "Post", fields)
		if err != nil {
			t.Fatalf("CreateRecord failed: %v", err)
		}

		if pk != float64(3) {
			t.Errorf("Expected PK 3, got %v", pk)
		}
	})

	t.Run("CreateRecordError", func(t *testing.T) {
		mockProj.mockResponse = `{"error": "Invalid field", "success": false}`

		fields := map[string]interface{}{
			"invalid_field": "value",
		}

		_, err := dv.CreateRecord("blog", "Post", fields)
		if err == nil {
			t.Error("Expected error for invalid field")
		}
	})

	t.Run("UpdateRecord", func(t *testing.T) {
		mockProj.mockResponse = `{"success": true}`

		fields := map[string]interface{}{
			"title": "Updated Title",
		}

		err := dv.UpdateRecord("blog", "Post", 1, fields)
		if err != nil {
			t.Fatalf("UpdateRecord failed: %v", err)
		}
	})

	t.Run("UpdateRecordError", func(t *testing.T) {
		mockProj.mockResponse = `{"error": "Record not found", "success": false}`

		fields := map[string]interface{}{
			"title": "Updated",
		}

		err := dv.UpdateRecord("blog", "Post", 999, fields)
		if err == nil {
			t.Error("Expected error for nonexistent record")
		}
	})

	t.Run("DeleteRecord", func(t *testing.T) {
		mockProj.mockResponse = `{"success": true}`

		err := dv.DeleteRecord("blog", "Post", 1)
		if err != nil {
			t.Fatalf("DeleteRecord failed: %v", err)
		}
	})

	t.Run("DeleteRecordError", func(t *testing.T) {
		mockProj.mockResponse = `{"error": "Record not found", "success": false}`

		err := dv.DeleteRecord("blog", "Post", 999)
		if err == nil {
			t.Error("Expected error for nonexistent record")
		}
	})

	t.Run("GetModelFields", func(t *testing.T) {
		mockProj.mockResponse = `{
			"fields": [
				{"name": "id", "type": "AutoField", "primary_key": true, "null": false},
				{"name": "title", "type": "CharField", "max_length": 200, "null": false},
				{"name": "content", "type": "TextField", "null": false}
			]
		}`

		fields, err := dv.GetModelFields("blog", "Post")
		if err != nil {
			t.Fatalf("GetModelFields failed: %v", err)
		}

		if len(fields) != 3 {
			t.Errorf("Expected 3 fields, got %d", len(fields))
		}

		if fields[0]["name"] != "id" {
			t.Errorf("Expected first field 'id', got '%v'", fields[0]["name"])
		}
	})

	t.Run("SearchRecords", func(t *testing.T) {
		mockProj.mockResponse = `{
			"records": [
				{"pk": 1, "model": "blog.Post", "fields": {"title": "Django Tutorial", "content": "Learn Django"}}
			],
			"total": 1,
			"page": 1,
			"page_size": 50,
			"has_next": false,
			"has_prev": false
		}`

		result, err := dv.SearchRecords("blog", "Post", "Django", 1, 50)
		if err != nil {
			t.Fatalf("SearchRecords failed: %v", err)
		}

		if len(result.Records) != 1 {
			t.Errorf("Expected 1 search result, got %d", len(result.Records))
		}
	})

	t.Run("GetRelatedObjects", func(t *testing.T) {
		mockProj.mockResponse = `{
			"records": [
				{"pk": 1, "model": "blog.Comment", "fields": {"content": "Great post!", "author": "user1"}},
				{"pk": 2, "model": "blog.Comment", "fields": {"content": "Thanks!", "author": "user2"}}
			]
		}`

		records, err := dv.GetRelatedObjects("blog", "Post", 1, "comment_set")
		if err != nil {
			t.Fatalf("GetRelatedObjects failed: %v", err)
		}

		if len(records) != 2 {
			t.Errorf("Expected 2 related objects, got %d", len(records))
		}
	})

	t.Run("BuildFilterCode", func(t *testing.T) {
		filters := map[string]string{
			"is_published": "true",
			"author":       "admin",
		}

		code := dv.buildFilterCode(filters)
		if code == "" {
			t.Error("Expected filter code to be generated")
		}

		// Should contain filter statements
		if !contains(code, "qs.filter") {
			t.Error("Filter code should contain 'qs.filter'")
		}
	})

	t.Run("EmptyFilters", func(t *testing.T) {
		code := dv.buildFilterCode(map[string]string{})
		if code != "" {
			t.Error("Expected empty filter code for no filters")
		}
	})
}

// TestDataViewerEdgeCases tests error handling and edge cases
func TestDataViewerEdgeCases(t *testing.T) {
	mockProj := &MockProject{}
	dv := NewDataViewer(mockProj)

	t.Run("InvalidJSON", func(t *testing.T) {
		mockProj.mockResponse = `invalid json`
		_, err := dv.QueryModel("blog", "Post", nil, 1, 50)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})

	t.Run("DefaultPagination", func(t *testing.T) {
		mockProj.mockResponse = `{
			"records": [],
			"total": 0,
			"page": 1,
			"page_size": 50,
			"has_next": false,
			"has_prev": false
		}`

		// Test with invalid page/pageSize
		result, err := dv.QueryModel("blog", "Post", nil, 0, 0)
		if err != nil {
			t.Fatalf("QueryModel failed: %v", err)
		}

		// Should default to page 1, pageSize 50
		if result.Page != 1 {
			t.Error("Expected default page 1")
		}
	})

	t.Run("EscapeSingleQuotes", func(t *testing.T) {
		filters := map[string]string{
			"title": "It's a test",
		}

		code := dv.buildFilterCode(filters)
		if !contains(code, "json.loads") {
			t.Error("Filter code should parse JSON payload")
		}
		if !contains(code, "qs.filter(**{key: value})") {
			t.Error("Filter code should apply dynamic filters safely")
		}
	})
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkDataViewer benchmarks data viewer operations
func BenchmarkDataViewer(b *testing.B) {
	mockProj := &MockProject{
		mockResponse: `{
			"records": [{"pk": 1, "model": "blog.Post", "fields": {"title": "Test"}}],
			"total": 1,
			"page": 1,
			"page_size": 50,
			"has_next": false,
			"has_prev": false
		}`,
	}
	dv := NewDataViewer(mockProj)

	b.Run("QueryModel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dv.QueryModel("blog", "Post", nil, 1, 50)
		}
	})

	b.Run("BuildFilterCode", func(b *testing.B) {
		filters := map[string]string{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		}
		for i := 0; i < b.N; i++ {
			dv.buildFilterCode(filters)
		}
	})
}

// TestDataViewerIntegration tests with real Django project (requires demo-project)
func TestDataViewerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This would test against actual demo-project
	// Requires Django to be running
	t.Log("Integration tests require running Django instance")
}
