package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ValidationError struct {
	Line    int
	Field   string
	Message string
}

func (e ValidationError) Format(filename string) string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d%s", filename, e.Line, e.Message)
	}
	return fmt.Sprintf("%s %s", filename, e.Message)
}

var (
	snakeCaseRegex = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	imageRegex     = regexp.MustCompile(`^registry\.bigbrother\.io/[^:]+:.+$`)
	memoryRegex    = regexp.MustCompile(`^[0-9]+(Gi|Mi|Ki)$`)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <yaml-file>\n", os.Args[0])
		os.Exit(1)
	}

	filename := os.Args[1]
	errors := validateYAMLFile(filename)

	if len(errors) > 0 {
		for _, err := range errors {
			fmt.Fprintln(os.Stderr, err.Format(filename))
		}
		os.Exit(1)
	}

	os.Exit(0)
}

func validateYAMLFile(filename string) []ValidationError {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []ValidationError{{
			Line:    0,
			Field:   "",
			Message: fmt.Sprintf(" cannot read file: %v", err),
		}}
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return []ValidationError{{
			Line:    0,
			Field:   "",
			Message: fmt.Sprintf(" cannot parse YAML: %v", err),
		}}
	}

	if len(root.Content) == 0 {
		return []ValidationError{{
			Line:    0,
			Field:   "",
			Message: " empty YAML document",
		}}
	}

	return validateDocument(root.Content[0])
}

func validateDocument(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "",
			Message: " root must be a mapping",
		}}
	}

	return validateTopLevelFields(node)
}

func validateTopLevelFields(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	// Собираем все поля
	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			if keyNode := node.Content[i]; keyNode.Kind == yaml.ScalarNode {
				fields[keyNode.Value] = node.Content[i+1]
			}
		}
	}

	// Проверяем обязательные поля
	for _, field := range []string{"apiVersion", "kind", "metadata", "spec"} {
		if _, found := fields[field]; !found {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   field,
				Message: fmt.Sprintf(" %s is required", field),
			})
		}
	}

	// Валидируем поля
	if apiVersionNode, found := fields["apiVersion"]; found {
		errors = append(errors, validateAPIVersion(apiVersionNode)...)
	}
	if kindNode, found := fields["kind"]; found {
		errors = append(errors, validateKind(kindNode)...)
	}
	if metadataNode, found := fields["metadata"]; found {
		errors = append(errors, validateMetadata(metadataNode)...)
	}
	if specNode, found := fields["spec"]; found {
		errors = append(errors, validateSpec(specNode)...)
	}

	return errors
}

func validateAPIVersion(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: " must be string",
		}}
	}
	if node.Value != "v1" {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: fmt.Sprintf(" has unsupported value '%s'", node.Value),
		}}
	}
	return nil
}

func validateKind(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "kind",
			Message: " must be string",
		}}
	}
	if node.Value != "Pod" {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "kind",
			Message: fmt.Sprintf(" has unsupported value '%s'", node.Value),
		}}
	}
	return nil
}

func validateMetadata(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "metadata",
			Message: " must be mapping",
		}}
	}

	// Ищем поле name
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" {
				valueNode := node.Content[i+1]
				if valueNode.Kind != yaml.ScalarNode {
					return []ValidationError{{
						Line:    valueNode.Line,
						Field:   "name",
						Message: " must be string",
					}}
				}
				if valueNode.Value == "" {
					return []ValidationError{{
						Line:    valueNode.Line,
						Field:   "name",
						Message: " is required",
					}}
				}
				return nil
			}
		}
	}

	return []ValidationError{{
		Line:    node.Line,
		Field:   "name",
		Message: " is required",
	}}
}

func validateSpec(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "spec",
			Message: " must be mapping",
		}}
	}

	var errors []ValidationError
	fields := make(map[string]*yaml.Node)

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			if keyNode := node.Content[i]; keyNode.Kind == yaml.ScalarNode {
				fields[keyNode.Value] = node.Content[i+1]
			}
		}
	}

	if containersNode, found := fields["containers"]; !found {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " is required",
		})
	} else {
		errors = append(errors, validateContainers(containersNode)...)
	}

	if osNode, found := fields["os"]; found {
		errors = append(errors, validateOS(osNode)...)
	}

	return errors
}

func validateOS(node *yaml.Node) []ValidationError {
	if node.Kind == yaml.ScalarNode {
		if node.Value != "linux" && node.Value != "windows" {
			return []ValidationError{{
				Line:    node.Line,
				Field:   "os",
				Message: fmt.Sprintf(" os has unsupported value '%s'", node.Value),
			}}
		}
	}
	return nil
}

func validateContainers(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.SequenceNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " must be list",
		}}
	}

	var errors []ValidationError

	for i, containerNode := range node.Content {
		errors = append(errors, validateContainer(containerNode, i)...)
	}

	return errors
}

func validateContainer(node *yaml.Node, index int) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "container",
			Message: " must be mapping",
		}}
	}

	var errors []ValidationError
	fields := make(map[string]*yaml.Node)

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			if keyNode := node.Content[i]; keyNode.Kind == yaml.ScalarNode {
				fields[keyNode.Value] = node.Content[i+1]
			}
		}
	}

	// Проверяем обязательные поля
	for _, field := range []string{"name", "image", "resources"} {
		if _, found := fields[field]; !found {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   field,
				Message: " is required",
			})
		}
	}

	// Валидируем поля
	for fieldName, fieldNode := range fields {
		switch fieldName {
		case "name":
			errors = append(errors, validateContainerName(fieldNode)...)
		case "image":
			errors = append(errors, validateImage(fieldNode)...)
		case "ports":
			errors = append(errors, validatePorts(fieldNode)...)
		case "readinessProbe", "livenessProbe":
			errors = append(errors, validateProbe(fieldNode, fieldName)...)
		case "resources":
			errors = append(errors, validateResources(fieldNode)...)
		}
	}

	return errors
}

func validateContainerName(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "name",
			Message: " must be string",
		}}
	}

	if node.Value == "" {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "name",
			Message: " is required",
		}}
	}

	if !snakeCaseRegex.MatchString(node.Value) {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "name",
			Message: fmt.Sprintf(" has invalid format '%s'", node.Value),
		}}
	}

	return nil
}

func validateImage(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "image",
			Message: " must be string",
		}}
	}

	if !imageRegex.MatchString(node.Value) {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "image",
			Message: fmt.Sprintf(" has invalid format '%s'", node.Value),
		}}
	}

	return nil
}

func validatePorts(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.SequenceNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "ports",
			Message: " must be list",
		}}
	}

	var errors []ValidationError

	for _, portNode := range node.Content {
		errors = append(errors, validatePort(portNode)...)
	}

	return errors
}

func validatePort(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "port",
			Message: " must be mapping",
		}}
	}

	// Ищем containerPort
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "containerPort" {
				valueNode := node.Content[i+1]
				return validateContainerPort(valueNode)
			}
		}
	}

	return []ValidationError{{
		Line:    node.Line,
		Field:   "containerPort",
		Message: " is required",
	}}
}

func validateContainerPort(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " must be integer",
		}}
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " must be integer",
		}}
	}

	if port <= 0 || port >= 65536 {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " value out of range",
		}}
	}

	return nil
}

func validateProbe(node *yaml.Node, probeType string) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   probeType,
			Message: " must be mapping",
		}}
	}

	// Ищем httpGet
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "httpGet" {
				valueNode := node.Content[i+1]
				return validateHTTPGetAction(valueNode)
			}
		}
	}

	return []ValidationError{{
		Line:    node.Line,
		Field:   "httpGet",
		Message: " is required",
	}}
}

func validateHTTPGetAction(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "httpGet",
			Message: " must be mapping",
		}}
	}

	var errors []ValidationError
	var foundPath, foundPort bool

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			if keyNode.Kind != yaml.ScalarNode {
				continue
			}

			valueNode := node.Content[i+1]

			switch keyNode.Value {
			case "path":
				foundPath = true
				if valueNode.Kind != yaml.ScalarNode {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "path",
						Message: " must be string",
					})
				} else if !strings.HasPrefix(valueNode.Value, "/") {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "path",
						Message: fmt.Sprintf(" has invalid format '%s'", valueNode.Value),
					})
				}
			case "port":
				foundPort = true
				errors = append(errors, validateProbePort(valueNode)...)
			}
		}
	}

	if !foundPath {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "path",
			Message: " is required",
		})
	}

	if !foundPort {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "port",
			Message: " is required",
		})
	}

	return errors
}

func validateProbePort(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.ScalarNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "port",
			Message: " must be integer",
		}}
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "port",
			Message: " must be integer",
		}}
	}

	if port <= 0 || port >= 65536 {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "port",
			Message: " value out of range",
		}}
	}

	return nil
}

func validateResources(node *yaml.Node) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   "resources",
			Message: " must be mapping",
		}}
	}

	var errors []ValidationError

	// Проверяем, что есть хотя бы одно из полей
	hasRequests := false
	hasLimits := false

	for i := 0; i < len(node.Content); i += 2 {
		if i < len(node.Content) {
			if keyNode := node.Content[i]; keyNode.Kind == yaml.ScalarNode {
				if keyNode.Value == "requests" {
					hasRequests = true
				} else if keyNode.Value == "limits" {
					hasLimits = true
				}
			}
		}
	}

	if !hasRequests && !hasLimits {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "resources",
			Message: " must contain at least one of: requests, limits",
		})
	}

	// Проверяем requests и limits
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			if keyNode.Kind == yaml.ScalarNode {
				if keyNode.Value == "requests" || keyNode.Value == "limits" {
					valueNode := node.Content[i+1]
					errors = append(errors, validateResourceMap(valueNode, keyNode.Value)...)
				}
			}
		}
	}

	return errors
}

func validateResourceMap(node *yaml.Node, resourceType string) []ValidationError {
	if node.Kind != yaml.MappingNode {
		return []ValidationError{{
			Line:    node.Line,
			Field:   resourceType,
			Message: " must be mapping",
		}}
	}

	var errors []ValidationError

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 < len(node.Content) {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Kind != yaml.ScalarNode {
				continue
			}

			switch keyNode.Value {
			case "cpu":
				if valueNode.Kind != yaml.ScalarNode {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "cpu",
						Message: " must be integer",
					})
				} else if valueNode.Tag != "!!int" {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "cpu",
						Message: " must be integer",
					})
				}
			case "memory":
				if valueNode.Kind != yaml.ScalarNode {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "memory",
						Message: " must be string",
					})
				} else if !memoryRegex.MatchString(valueNode.Value) {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "memory",
						Message: fmt.Sprintf(" has invalid format '%s'", valueNode.Value),
					})
				}
			default:
				errors = append(errors, ValidationError{
					Line:    keyNode.Line,
					Field:   keyNode.Value,
					Message: fmt.Sprintf(" has unsupported value '%s'", keyNode.Value),
				})
			}
		}
	}

	return errors
}
