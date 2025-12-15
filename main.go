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
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "",
			Message: " root must be a mapping",
		})
		return errors
	}

	errors = append(errors, validateTopLevelFields(node)...)

	return errors
}

func validateTopLevelFields(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

		switch keyNode.Value {
		case "apiVersion":
			errors = append(errors, validateAPIVersion(valueNode)...)
		case "kind":
			errors = append(errors, validateKind(valueNode)...)
		case "metadata":
			errors = append(errors, validateMetadata(valueNode)...)
		case "spec":
			errors = append(errors, validateSpec(valueNode)...)
		}
	}

	// Проверяем обязательные поля
	fields := []string{"apiVersion", "kind", "metadata", "spec"}
	for _, field := range fields {
		found := false
		for i := 0; i < len(node.Content); i += 2 {
			if i >= len(node.Content) {
				continue
			}
			if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == field {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   field,
				Message: fmt.Sprintf(" %s is required", field),
			})
		}
	}

	return errors
}

func validateAPIVersion(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: " must be string",
		})
		return errors
	}

	if node.Value != "v1" {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: fmt.Sprintf(" has unsupported value '%s'", node.Value),
		})
	}

	return errors
}

func validateKind(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "kind",
			Message: " must be string",
		})
		return errors
	}

	if node.Value != "Pod" {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "kind",
			Message: fmt.Sprintf(" has unsupported value '%s'", node.Value),
		})
	}

	return errors
}

func validateMetadata(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "metadata",
			Message: " must be mapping",
		})
		return errors
	}

	// Ищем поле name
	foundName := false
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" {
			foundName = true
			if valueNode.Kind != yaml.ScalarNode {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   "name",
					Message: " must be string",
				})
			} else if valueNode.Value == "" {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   "name",
					Message: " is required",
				})
			}
			break
		}
	}

	if !foundName {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "name",
			Message: " is required",
		})
	}

	return errors
}

func validateSpec(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec",
			Message: " must be mapping",
		})
		return errors
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

		switch keyNode.Value {
		case "containers":
			errors = append(errors, validateContainers(valueNode)...)
		case "os":
			errors = append(errors, validateOS(valueNode)...)
		}
	}

	// Проверяем обязательное поле containers
	foundContainers := false
	for i := 0; i < len(node.Content); i += 2 {
		if i >= len(node.Content) {
			continue
		}
		if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == "containers" {
			foundContainers = true
			break
		}
	}

	if !foundContainers {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " is required",
		})
	}

	return errors
}

func validateOS(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind == yaml.ScalarNode {
		if node.Value != "linux" && node.Value != "windows" {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "os",
				Message: fmt.Sprintf(" os has unsupported value '%s'", node.Value),
			})
		}
	} else if node.Kind == yaml.MappingNode {
		// Ищем поле name
		foundName := false
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 >= len(node.Content) {
				continue
			}
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" {
				foundName = true
				if valueNode.Kind != yaml.ScalarNode {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "os",
						Message: " must be string",
					})
				} else if valueNode.Value != "linux" && valueNode.Value != "windows" {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "os",
						Message: fmt.Sprintf(" os has unsupported value '%s'", valueNode.Value),
					})
				}
				break
			}
		}

		if !foundName {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "os",
				Message: " is required",
			})
		}
	}

	return errors
}

func validateContainers(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.SequenceNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " must be list",
		})
		return errors
	}

	if len(node.Content) == 0 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " must contain at least one container",
		})
		return errors
	}

	for idx, containerNode := range node.Content {
		errors = append(errors, validateContainer(containerNode, idx)...)
	}

	// Проверяем уникальность имен контейнеров
	containerNames := make(map[string]int)
	for idx, containerNode := range node.Content {
		if containerNode.Kind == yaml.MappingNode {
			for i := 0; i < len(containerNode.Content); i += 2 {
				if i+1 >= len(containerNode.Content) {
					continue
				}
				keyNode := containerNode.Content[i]
				valueNode := containerNode.Content[i+1]

				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
					name := valueNode.Value
					if prevIdx, exists := containerNames[name]; exists {
						errors = append(errors, ValidationError{
							Line:    valueNode.Line,
							Field:   fmt.Sprintf("spec.containers[%d].name", idx),
							Message: fmt.Sprintf(" duplicate container name, first used at index %d", prevIdx),
						})
					} else {
						containerNames[name] = idx
					}
				}
			}
		}
	}

	return errors
}

func validateContainer(node *yaml.Node, index int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d]", index),
			Message: " must be mapping",
		})
		return errors
	}

	// Проверяем обязательные поля
	requiredFields := []string{"name", "image", "resources"}
	for _, field := range requiredFields {
		found := false
		for i := 0; i < len(node.Content); i += 2 {
			if i >= len(node.Content) {
				continue
			}
			if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == field {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   fmt.Sprintf("spec.containers[%d].%s", index, field),
				Message: " is required",
			})
		}
	}

	// Проверяем все поля контейнера
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

		switch keyNode.Value {
		case "name":
			errors = append(errors, validateContainerName(valueNode, index)...)
		case "image":
			errors = append(errors, validateImage(valueNode, index)...)
		case "ports":
			errors = append(errors, validatePorts(valueNode, index)...)
		case "readinessProbe", "livenessProbe":
			errors = append(errors, validateProbe(valueNode, index, keyNode.Value)...)
		case "resources":
			errors = append(errors, validateResources(valueNode, index)...)
		}
	}

	return errors
}

func validateContainerName(node *yaml.Node, index int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].name", index),
			Message: " must be string",
		})
		return errors
	}

if node.Value == "" {
    errors = append(errors, ValidationError{
        Line:    node.Line,
        Field:   fmt.Sprintf("spec.containers[%d].name", index),
        Message: " name is required",  // ← с пробелом и словом "name" в начале
    })
}
	} else if !snakeCaseRegex.MatchString(node.Value) {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].name", index),
			Message: fmt.Sprintf(" has invalid format '%s'", node.Value),
		})
	}

	return errors
}

func validateImage(node *yaml.Node, index int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].image", index),
			Message: " must be string",
		})
		return errors
	}

	if !imageRegex.MatchString(node.Value) {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].image", index),
			Message: fmt.Sprintf(" has invalid format '%s'", node.Value),
		})
	}

	return errors
}

func validatePorts(node *yaml.Node, containerIndex int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.SequenceNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].ports", containerIndex),
			Message: " must be list",
		})
		return errors
	}

	for idx, portNode := range node.Content {
		errors = append(errors, validatePort(portNode, containerIndex, idx)...)
	}

	return errors
}

func validatePort(node *yaml.Node, containerIndex, portIndex int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].ports[%d]", containerIndex, portIndex),
			Message: " must be mapping",
		})
		return errors
	}

	// Ищем containerPort
	foundContainerPort := false
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "containerPort" {
			foundContainerPort = true
			errors = append(errors, validateContainerPort(valueNode, containerIndex, portIndex)...)
			break
		}
	}

	if !foundContainerPort {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].ports[%d].containerPort", containerIndex, portIndex),
			Message: " is required",
		})
	}

	return errors
}

func validateContainerPort(node *yaml.Node, containerIndex, portIndex int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " must be integer",
		})
		return errors
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " must be integer",
		})
		return errors
	}

	if port <= 0 || port >= 65536 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containerPort",
			Message: " value out of range",
		})
	}

	return errors
}

func validateProbe(node *yaml.Node, containerIndex int, probeType string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s", containerIndex, probeType),
			Message: " must be mapping",
		})
		return errors
	}

	// Ищем httpGet
	foundHTTPGet := false
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "httpGet" {
			foundHTTPGet = true
			errors = append(errors, validateHTTPGetAction(valueNode, containerIndex, probeType)...)
			break
		}
	}

	if !foundHTTPGet {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet", containerIndex, probeType),
			Message: " is required",
		})
	}

	return errors
}

func validateHTTPGetAction(node *yaml.Node, containerIndex int, probeType string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet", containerIndex, probeType),
			Message: " must be mapping",
		})
		return errors
	}

	// Ищем path и port
	foundPath := false
	foundPort := false

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

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
			errors = append(errors, validateProbePort(valueNode, containerIndex, probeType)...)
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

func validateProbePort(node *yaml.Node, containerIndex int, probeType string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "port",
			Message: " must be integer",
		})
		return errors
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "port",
			Message: " must be integer",
		})
		return errors
	}

	if port <= 0 || port >= 65536 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.port", containerIndex, probeType),
			Message: " port value out of range",
		})
	}

	return errors
}

func validateResources(node *yaml.Node, containerIndex int) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].resources", containerIndex),
			Message: " must be mapping",
		})
		return errors
	}

	// Проверяем, что есть хотя бы одно из полей
	hasRequests := false
	hasLimits := false

	for i := 0; i < len(node.Content); i += 2 {
		if i >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		if keyNode.Kind == yaml.ScalarNode {
			if keyNode.Value == "requests" {
				hasRequests = true
			} else if keyNode.Value == "limits" {
				hasLimits = true
			}
		}
	}

	if !hasRequests && !hasLimits {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].resources", containerIndex),
			Message: " must contain at least one of: requests, limits",
		})
	}

	// Проверяем requests и limits
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

		if keyNode.Value == "requests" || keyNode.Value == "limits" {
			errors = append(errors, validateResourceMap(valueNode, containerIndex, keyNode.Value)...)
		}
	}

	return errors
}

func validateResourceMap(node *yaml.Node, containerIndex int, resourceType string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].resources.%s", containerIndex, resourceType),
			Message: " must be mapping",
		})
		return errors
	}

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
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

	return errors
}
