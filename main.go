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

	return validateTopLevelFields(node)
}

func validateTopLevelFields(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	fields := make(map[string]*yaml.Node)

	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	requiredFields := []string{"apiVersion", "kind", "metadata", "spec"}
	for _, field := range requiredFields {
		if _, exists := fields[field]; !exists {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   field,
				Message: fmt.Sprintf(" %s is required", field),
			})
		}
	}

	for name, valueNode := range fields {
		switch name {
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	if nameNode, exists := fields["name"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "name",
			Message: " is required",
		})
	} else if nameNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    nameNode.Line,
			Field:   "name",
			Message: " must be string",
		})
	} else if nameNode.Value == "" {
		errors = append(errors, ValidationError{
			Line:    nameNode.Line,
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	if containersNode, exists := fields["containers"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: " is required",
		})
	} else {
		errors = append(errors, validateContainers(containersNode)...)
	}

	if osNode, exists := fields["os"]; exists {
		errors = append(errors, validateOS(osNode)...)
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
		fields := make(map[string]*yaml.Node)
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 >= len(node.Content) {
				continue
			}
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Kind == yaml.ScalarNode {
				fields[keyNode.Value] = valueNode
			}
		}

		if nameNode, exists := fields["name"]; !exists {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "os",
				Message: " is required",
			})
		} else if nameNode.Kind != yaml.ScalarNode {
			errors = append(errors, ValidationError{
				Line:    nameNode.Line,
				Field:   "os",
				Message: " must be string",
			})
		} else if nameNode.Value != "linux" && nameNode.Value != "windows" {
			errors = append(errors, ValidationError{
				Line:    nameNode.Line,
				Field:   "os",
				Message: fmt.Sprintf(" os has unsupported value '%s'", nameNode.Value),
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

	containerNames := make(map[string]bool)

	for idx, containerNode := range node.Content {
		containerErrors := validateContainer(containerNode, idx)
		errors = append(errors, containerErrors...)

		if containerNode.Kind == yaml.MappingNode {
			for i := 0; i < len(containerNode.Content); i += 2 {
				if i+1 >= len(containerNode.Content) {
					continue
				}
				keyNode := containerNode.Content[i]
				valueNode := containerNode.Content[i+1]

				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
					name := valueNode.Value
					if containerNames[name] {
						errors = append(errors, ValidationError{
							Line:    valueNode.Line,
							Field:   fmt.Sprintf("spec.containers[%d].name", idx),
							Message: fmt.Sprintf(" duplicate container name '%s'", name),
						})
					}
					containerNames[name] = true
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	requiredFields := []string{"name", "image", "resources"}
	for _, field := range requiredFields {
		if _, exists := fields[field]; !exists {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   fmt.Sprintf("spec.containers[%d].%s", index, field),
				Message: " is required",
			})
		}
	}

	for fieldName, fieldNode := range fields {
		switch fieldName {
		case "name":
			errors = append(errors, validateContainerName(fieldNode, index)...)
		case "image":
			errors = append(errors, validateImage(fieldNode, index)...)
		case "ports":
			errors = append(errors, validatePorts(fieldNode, index)...)
		case "readinessProbe", "livenessProbe":
			errors = append(errors, validateProbe(fieldNode, index, fieldName)...)
		case "resources":
			errors = append(errors, validateResources(fieldNode, index)...)
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
			Message: " is required",
		})
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	if containerPortNode, exists := fields["containerPort"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].ports[%d].containerPort", containerIndex, portIndex),
			Message: " is required",
		})
	} else {
		errors = append(errors, validatePortNumber(containerPortNode, containerIndex, portIndex)...)
	}

	return errors
}

func validatePortNumber(node *yaml.Node, containerIndex, portIndex int) []ValidationError {
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	if httpGetNode, exists := fields["httpGet"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet", containerIndex, probeType),
			Message: " is required",
		})
	} else {
		errors = append(errors, validateHTTPGetAction(httpGetNode, containerIndex, probeType)...)
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	if pathNode, exists := fields["path"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.path", containerIndex, probeType),
			Message: " is required",
		})
	} else if pathNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    pathNode.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.path", containerIndex, probeType),
			Message: " must be string",
		})
	} else if !strings.HasPrefix(pathNode.Value, "/") {
		errors = append(errors, ValidationError{
			Line:    pathNode.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.path", containerIndex, probeType),
			Message: fmt.Sprintf(" has invalid format '%s'", pathNode.Value),
		})
	}

	if portNode, exists := fields["port"]; !exists {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.port", containerIndex, probeType),
			Message: " is required",
		})
	} else {
		errors = append(errors, validateProbePort(portNode, containerIndex, probeType)...)
	}

	return errors
}

func validateProbePort(node *yaml.Node, containerIndex int, probeType string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.port", containerIndex, probeType),
			Message: " must be integer",
		})
		return errors
	}

	port, err := strconv.Atoi(node.Value)
	if err != nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.port", containerIndex, probeType),
			Message: " must be integer",
		})
		return errors
	}

	if port <= 0 || port >= 65536 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("spec.containers[%d].%s.httpGet.port", containerIndex, probeType),
			Message: " value out of range",
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

	fields := make(map[string]*yaml.Node)
	for i := 0; i < len(node.Content); i += 2 {
		if i+1 >= len(node.Content) {
			continue
		}
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode {
			fields[keyNode.Value] = valueNode
		}
	}

	// Проверяем, что есть хотя бы одно из полей
	if _, hasRequests := fields["requests"]; !hasRequests {
		if _, hasLimits := fields["limits"]; !hasLimits {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   fmt.Sprintf("spec.containers[%d].resources", containerIndex),
				Message: " must contain at least one of: requests, limits",
			})
		}
	}

	for fieldName, fieldNode := range fields {
		if fieldName == "requests" || fieldName == "limits" {
			errors = append(errors, validateResourceMap(fieldNode, containerIndex, fieldName)...)
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

		resourceName := keyNode.Value
		fieldPrefix := fmt.Sprintf("spec.containers[%d].resources.%s.%s", containerIndex, resourceType, resourceName)

		switch resourceName {
		case "cpu":
			// CPU должно быть целым числом
			if valueNode.Kind != yaml.ScalarNode {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   fieldPrefix,
					Message: " must be integer",
				})
			} else if valueNode.Tag != "!!int" {
				// Проверяем тег YAML - должен быть !!int для целых чисел
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   fieldPrefix,
					Message: " must be integer",
				})
			}
		case "memory":
			if valueNode.Kind != yaml.ScalarNode {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   fieldPrefix,
					Message: " must be string",
				})
			} else if !memoryRegex.MatchString(valueNode.Value) {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   fieldPrefix,
					Message: fmt.Sprintf(" has invalid format '%s'", valueNode.Value),
				})
			}
		default:
			errors = append(errors, ValidationError{
				Line:    keyNode.Line,
				Field:   fieldPrefix,
				Message: fmt.Sprintf(" has unsupported value '%s'", resourceName),
			})
		}
	}

	return errors
}
