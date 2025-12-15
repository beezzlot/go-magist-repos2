package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Структуры для хранения позиций
type ValidationError struct {
	Line    int
	Field   string
	Message string
}

func (e ValidationError) Format(filename string) string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d %s", filename, e.Line, e.Message)
	}
	return fmt.Sprintf("%s %s", filename, e.Message)
}

// Константы и регулярные выражения
var (
	snakeCaseRegex = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	imageRegex     = regexp.MustCompile(`^registry\.bigbrother\.io/[^:]+:[^:]+$`)
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
			Message: fmt.Sprintf("cannot read file: %v", err),
		}}
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return []ValidationError{{
			Line:    0,
			Field:   "",
			Message: fmt.Sprintf("cannot parse YAML: %v", err),
		}}
	}

	if len(root.Content) == 0 {
		return []ValidationError{{
			Line:    0,
			Field:   "",
			Message: "empty YAML document",
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
			Message: "root must be a mapping",
		})
		return errors
	}

	// Проверяем обязательные поля верхнего уровня
	errors = append(errors, validateTopLevelFields(node)...)
	
	return errors
}

func validateTopLevelFields(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	var foundFields = make(map[string]bool)
	
	// Проходим по парам ключ-значение
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		foundFields[fieldName] = true
		
		switch fieldName {
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
	requiredFields := []string{"apiVersion", "kind", "metadata", "spec"}
	for _, field := range requiredFields {
		if !foundFields[field] {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   field,
				Message: "is required",
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
			Message: "must be string",
		})
		return errors
	}
	
	if node.Value != "v1" {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: fmt.Sprintf("has unsupported value '%s'", node.Value),
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
			Message: "must be string",
		})
		return errors
	}
	
	if node.Value != "Pod" {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "kind",
			Message: fmt.Sprintf("has unsupported value '%s'", node.Value),
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
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundFields = make(map[string]bool)
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		foundFields[fieldName] = true
		
		switch fieldName {
		case "name":
			errors = append(errors, validateMetadataName(valueNode)...)
		case "namespace":
			errors = append(errors, validateStringField(valueNode, "namespace")...)
		case "labels":
			errors = append(errors, validateLabels(valueNode)...)
		}
	}
	
	// Проверяем обязательное поле name
	if !foundFields["name"] {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "metadata.name",
			Message: "is required",
		})
	}
	
	return errors
}

func validateMetadataName(node *yaml.Node) []ValidationError {
	return validateStringField(node, "metadata.name")
}

func validateStringField(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
	}
	
	return errors
}

func validateLabels(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "metadata.labels",
			Message: "must be mapping",
		})
		return errors
	}
	
	// Проверяем, что все значения - строки
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if valueNode.Kind != yaml.ScalarNode {
			errors = append(errors, ValidationError{
				Line:    valueNode.Line,
				Field:   fmt.Sprintf("metadata.labels.%s", keyNode.Value),
				Message: "must be string",
			})
		}
	}
	
	return errors
}

func validateSpec(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec",
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundFields = make(map[string]bool)
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		foundFields[fieldName] = true
		
		switch fieldName {
		case "os":
			errors = append(errors, validateOS(valueNode)...)
		case "containers":
			errors = append(errors, validateContainers(valueNode)...)
		}
	}
	
	// Проверяем обязательное поле containers
	if !foundFields["containers"] {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: "is required",
		})
	}
	
	return errors
}

func validateOS(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	
	// Поддерживается как строка (пример из задания), так и объект (по спецификации)
	if node.Kind == yaml.ScalarNode {
		if node.Value != "linux" && node.Value != "windows" {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "spec.os",
				Message: fmt.Sprintf("has unsupported value '%s'", node.Value),
			})
		}
	} else if node.Kind == yaml.MappingNode {
		// Объект PodOS с полем name
		errors = append(errors, validateOSObject(node)...)
	} else {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.os",
			Message: "must be string or mapping",
		})
	}
	
	return errors
}

func validateOSObject(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	
	var foundName bool
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		if keyNode.Value == "name" {
			foundName = true
			if valueNode.Kind != yaml.ScalarNode {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   "spec.os.name",
					Message: "must be string",
				})
			} else if valueNode.Value != "linux" && valueNode.Value != "windows" {
				errors = append(errors, ValidationError{
					Line:    valueNode.Line,
					Field:   "spec.os.name",
					Message: fmt.Sprintf("has unsupported value '%s'", valueNode.Value),
				})
			}
		}
	}
	
	if !foundName {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.os.name",
			Message: "is required",
		})
	}
	
	return errors
}

func validateContainers(node *yaml.Node) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.SequenceNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: "must be list",
		})
		return errors
	}
	
	if len(node.Content) == 0 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: "must contain at least one container",
		})
	}
	
	// Проверяем уникальность имен контейнеров
	containerNames := make(map[string]int)
	
	for idx, containerNode := range node.Content {
		prefix := fmt.Sprintf("spec.containers[%d]", idx)
		errors = append(errors, validateContainer(containerNode, prefix)...)
		
		// Извлекаем имя контейнера для проверки уникальности
		if containerNode.Kind == yaml.MappingNode {
			for i := 0; i < len(containerNode.Content); i += 2 {
				keyNode := containerNode.Content[i]
				valueNode := containerNode.Content[i+1]
				
				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
					name := valueNode.Value
					if prevIdx, exists := containerNames[name]; exists {
						errors = append(errors, ValidationError{
							Line:    valueNode.Line,
							Field:   fmt.Sprintf("spec.containers[%d].name", idx),
							Message: fmt.Sprintf("duplicate container name, first used at index %d", prevIdx),
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

func validateContainer(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundFields = make(map[string]bool)
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		foundFields[fieldName] = true
		fieldPrefix := fmt.Sprintf("%s.%s", prefix, fieldName)
		
		switch fieldName {
		case "name":
			errors = append(errors, validateContainerName(valueNode, fieldPrefix)...)
		case "image":
			errors = append(errors, validateImage(valueNode, fieldPrefix)...)
		case "ports":
			errors = append(errors, validatePorts(valueNode, fieldPrefix)...)
		case "readinessProbe":
			errors = append(errors, validateProbe(valueNode, fieldPrefix)...)
		case "livenessProbe":
			errors = append(errors, validateProbe(valueNode, fieldPrefix)...)
		case "resources":
			errors = append(errors, validateResources(valueNode, fieldPrefix)...)
		}
	}
	
	// Проверяем обязательные поля
	requiredFields := []string{"name", "image", "resources"}
	for _, field := range requiredFields {
		if !foundFields[field] {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   fmt.Sprintf("%s.%s", prefix, field),
				Message: "is required",
			})
		}
	}
	
	return errors
}

func validateContainerName(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
		return errors
	}
	
	if !snakeCaseRegex.MatchString(node.Value) {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: fmt.Sprintf("has invalid format '%s'", node.Value),
		})
	}
	
	return errors
}

func validateImage(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
		return errors
	}
	
	if !imageRegex.MatchString(node.Value) {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: fmt.Sprintf("has invalid format '%s'", node.Value),
		})
	}
	
	return errors
}

func validatePorts(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.SequenceNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be list",
		})
		return errors
	}
	
	for idx, portNode := range node.Content {
		portPrefix := fmt.Sprintf("%s[%d]", prefix, idx)
		errors = append(errors, validatePort(portNode, portPrefix)...)
	}
	
	return errors
}

func validatePort(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundContainerPort bool
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		fieldPrefix := fmt.Sprintf("%s.%s", prefix, fieldName)
		
		switch fieldName {
		case "containerPort":
			foundContainerPort = true
			errors = append(errors, validatePortNumber(valueNode, fieldPrefix)...)
		case "protocol":
			errors = append(errors, validateProtocol(valueNode, fieldPrefix)...)
		}
	}
	
	if !foundContainerPort {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("%s.containerPort", prefix),
			Message: "is required",
		})
	}
	
	return errors
}

func validatePortNumber(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be integer",
		})
		return errors
	}
	
	port, err := strconv.Atoi(node.Value)
	if err != nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be integer",
		})
		return errors
	}
	
	if port <= 0 || port >= 65536 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "value out of range",
		})
	}
	
	return errors
}

func validateProtocol(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
		return errors
	}
	
	if node.Value != "TCP" && node.Value != "UDP" {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: fmt.Sprintf("has unsupported value '%s'", node.Value),
		})
	}
	
	return errors
}

func validateProbe(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundHTTPGet bool
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		if keyNode.Value == "httpGet" {
			foundHTTPGet = true
			errors = append(errors, validateHTTPGetAction(valueNode, fmt.Sprintf("%s.httpGet", prefix))...)
		}
	}
	
	if !foundHTTPGet {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fmt.Sprintf("%s.httpGet", prefix),
			Message: "is required",
		})
	}
	
	return errors
}

func validateHTTPGetAction(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	var foundFields = make(map[string]bool)
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		fieldName := keyNode.Value
		foundFields[fieldName] = true
		fieldPrefix := fmt.Sprintf("%s.%s", prefix, fieldName)
		
		switch fieldName {
		case "path":
			errors = append(errors, validateProbePath(valueNode, fieldPrefix)...)
		case "port":
			errors = append(errors, validatePortNumber(valueNode, fieldPrefix)...)
		}
	}
	
	// Проверяем обязательные поля
	requiredFields := []string{"path", "port"}
	for _, field := range requiredFields {
		if !foundFields[field] {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   fmt.Sprintf("%s.%s", prefix, field),
				Message: "is required",
			})
		}
	}
	
	return errors
}

func validateProbePath(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
		return errors
	}
	
	if !strings.HasPrefix(node.Value, "/") {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: fmt.Sprintf("has invalid format '%s'", node.Value),
		})
	}
	
	return errors
}

func validateResources(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	// Проверяем, что есть хотя бы одно из полей requests или limits
	hasRequests := false
	hasLimits := false
	
	for i := 0; i < len(node.Content); i += 2 {
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
			Field:   prefix,
			Message: "must contain at least one of: requests, limits",
		})
	}
	
	// Валидируем отдельно requests и limits
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		if keyNode.Value == "requests" || keyNode.Value == "limits" {
			errors = append(errors, validateResourceMap(valueNode, fmt.Sprintf("%s.%s", prefix, keyNode.Value))...)
		}
	}
	
	return errors
}

func validateResourceMap(node *yaml.Node, prefix string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   prefix,
			Message: "must be mapping",
		})
		return errors
	}
	
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		
		resourceName := keyNode.Value
		fieldPrefix := fmt.Sprintf("%s.%s", prefix, resourceName)
		
		switch resourceName {
		case "cpu":
			errors = append(errors, validateCPUResource(valueNode, fieldPrefix)...)
		case "memory":
			errors = append(errors, validateMemoryResource(valueNode, fieldPrefix)...)
		default:
			errors = append(errors, ValidationError{
				Line:    keyNode.Line,
				Field:   fieldPrefix,
				Message: fmt.Sprintf("has unsupported value '%s'", resourceName),
			})
		}
	}
	
	return errors
}

func validateCPUResource(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be integer",
		})
		return errors
	}
	
	if _, err := strconv.Atoi(node.Value); err != nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be integer",
		})
	}
	
	return errors
}

func validateMemoryResource(node *yaml.Node, fieldName string) []ValidationError {
	var errors []ValidationError
	
	if node.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: "must be string",
		})
		return errors
	}
	
	if !memoryRegex.MatchString(node.Value) {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   fieldName,
			Message: fmt.Sprintf("has invalid format '%s'", node.Value),
		})
	}
	
	return errors
}