package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError представляет ошибку валидации
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
	imageRegex     = regexp.MustCompile(`^registry\.bigbrother\.io/[^:]+:.+$`)
	memoryRegex    = regexp.MustCompile(`^[0-9]+(Gi|Mi|Ki)$`)
	portMin        = 1
	portMax        = 65535
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

	errors = append(errors, validateTopLevelFields(node)...)

	return errors
}

func getMap(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i < len(m.Content)-1; i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Value == key {
			return k, v
		}
	}
	return nil, nil
}

func validateTopLevelFields(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	// apiVersion
	_, apiNode := getMap(node, "apiVersion")
	if apiNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "apiVersion",
			Message: "is required",
		})
	} else if apiNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    apiNode.Line,
			Field:   "apiVersion",
			Message: "must be string",
		})
	} else if apiNode.Value != "v1" {
		errors = append(errors, ValidationError{
			Line:    apiNode.Line,
			Field:   "apiVersion",
			Message: fmt.Sprintf("has unsupported value '%s'", apiNode.Value),
		})
	}

	// kind
	_, kindNode := getMap(node, "kind")
	if kindNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "kind",
			Message: "is required",
		})
	} else if kindNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    kindNode.Line,
			Field:   "kind",
			Message: "must be string",
		})
	} else if kindNode.Value != "Pod" {
		errors = append(errors, ValidationError{
			Line:    kindNode.Line,
			Field:   "kind",
			Message: fmt.Sprintf("has unsupported value '%s'", kindNode.Value),
		})
	}

	// metadata
	_, metaNode := getMap(node, "metadata")
	if metaNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "metadata",
			Message: "is required",
		})
	} else {
		errors = append(errors, validateMetadata(metaNode)...)
	}

	// spec
	_, specNode := getMap(node, "spec")
	if specNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec",
			Message: "is required",
		})
	} else {
		errors = append(errors, validateSpec(specNode)...)
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

	// name
	_, nameNode := getMap(node, "name")
	if nameNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "metadata.name",
			Message: "is required",
		})
	} else if nameNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    nameNode.Line,
			Field:   "metadata.name",
			Message: "must be string",
		})
	} else if strings.TrimSpace(nameNode.Value) == "" {
		// Для пустой строки используем просто "name"
		errors = append(errors, ValidationError{
			Line:    nameNode.Line,
			Field:   "name",
			Message: "is required",
		})
	}

	// namespace (необязательное)
	if _, nsNode := getMap(node, "namespace"); nsNode != nil && nsNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    nsNode.Line,
			Field:   "metadata.namespace",
			Message: "must be string",
		})
	}

	// labels (необязательное)
	if _, labelsNode := getMap(node, "labels"); labelsNode != nil {
		if labelsNode.Kind != yaml.MappingNode {
			errors = append(errors, ValidationError{
				Line:    labelsNode.Line,
				Field:   "metadata.labels",
				Message: "must be mapping",
			})
		} else {
			// Проверяем, что все значения - строки
			for i := 0; i < len(labelsNode.Content)-1; i += 2 {
				valueNode := labelsNode.Content[i+1]
				if valueNode.Kind != yaml.ScalarNode {
					errors = append(errors, ValidationError{
						Line:    valueNode.Line,
						Field:   "metadata.labels",
						Message: "has invalid format ''",
					})
					break
				}
			}
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

	// os (необязательное)
	if _, osNode := getMap(node, "os"); osNode != nil {
		errors = append(errors, validateOS(osNode)...)
	}

	// containers (обязательное)
	_, containersNode := getMap(node, "containers")
	if containersNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: "is required",
		})
	} else {
		errors = append(errors, validateContainers(containersNode)...)
	}

	return errors
}

func validateOS(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind == yaml.ScalarNode {
		val := strings.ToLower(node.Value)
		if val != "linux" && val != "windows" {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "os",
				Message: fmt.Sprintf("has unsupported value '%s'", node.Value),
			})
		}
	} else if node.Kind == yaml.MappingNode {
		_, nameNode := getMap(node, "name")
		if nameNode == nil {
			errors = append(errors, ValidationError{
				Line:    node.Line,
				Field:   "spec.os.name",
				Message: "is required",
			})
		} else if nameNode.Kind != yaml.ScalarNode {
			errors = append(errors, ValidationError{
				Line:    nameNode.Line,
				Field:   "spec.os.name",
				Message: "must be string",
			})
		} else {
			val := strings.ToLower(nameNode.Value)
			if val != "linux" && val != "windows" {
				errors = append(errors, ValidationError{
					Line:    nameNode.Line,
					Field:   "os",
					Message: fmt.Sprintf("has unsupported value '%s'", nameNode.Value),
				})
			}
		}
	} else {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.os",
			Message: "must be object",
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
			Message: "must be array",
		})
		return errors
	}

	if len(node.Content) == 0 {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "spec.containers",
			Message: "must contain at least one container",
		})
		return errors
	}

	// Проверяем уникальность имен контейнеров
	containerNames := make(map[string]bool)

	for _, containerNode := range node.Content {
		containerErrors := validateContainer(containerNode)
		errors = append(errors, containerErrors...)

		// Извлекаем имя для проверки уникальности
		if containerNode.Kind == yaml.MappingNode {
			_, nameNode := getMap(containerNode, "name")
			if nameNode != nil && nameNode.Kind == yaml.ScalarNode && nameNode.Value != "" {
				if containerNames[nameNode.Value] {
					errors = append(errors, ValidationError{
						Line:    nameNode.Line,
						Field:   "containers.name",
						Message: fmt.Sprintf("has invalid format '%s'", nameNode.Value),
					})
				}
				containerNames[nameNode.Value] = true
			}
		}
	}

	return errors
}

func validateContainer(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "container",
			Message: "must be mapping",
		})
		return errors
	}

	// name (обязательное)
	_, nameNode := getMap(node, "name")
	if nameNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "name",
			Message: "is required",
		})
	} else if nameNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    nameNode.Line,
			Field:   "name",
			Message: "must be string",
		})
	} else {
		// Для пустой строки
		if strings.TrimSpace(nameNode.Value) == "" {
			errors = append(errors, ValidationError{
				Line:    nameNode.Line,
				Field:   "name",
				Message: "is required",
			})
		} else if !snakeCaseRegex.MatchString(nameNode.Value) {
			errors = append(errors, ValidationError{
				Line:    nameNode.Line,
				Field:   "containers.name",
				Message: fmt.Sprintf("has invalid format '%s'", nameNode.Value),
			})
		}
	}

	// image (обязательное)
	_, imageNode := getMap(node, "image")
	if imageNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containers.image",
			Message: "is required",
		})
	} else if imageNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    imageNode.Line,
			Field:   "containers.image",
			Message: "must be string",
		})
	} else if !imageRegex.MatchString(imageNode.Value) {
		errors = append(errors, ValidationError{
			Line:    imageNode.Line,
			Field:   "containers.image",
			Message: fmt.Sprintf("has invalid format '%s'", imageNode.Value),
		})
	}

	// ports (необязательное)
	if _, portsNode := getMap(node, "ports"); portsNode != nil {
		if portsNode.Kind != yaml.SequenceNode {
			errors = append(errors, ValidationError{
				Line:    portsNode.Line,
				Field:   "containers.ports",
				Message: "must be array",
			})
		} else {
			for _, portNode := range portsNode.Content {
				errors = append(errors, validateContainerPort(portNode)...)
			}
		}
	}

	// readinessProbe (необязательное)
	if _, probeNode := getMap(node, "readinessProbe"); probeNode != nil {
		errors = append(errors, validateProbe(probeNode, "containers.readinessProbe")...)
	}

	// livenessProbe (необязательное)
	if _, probeNode := getMap(node, "livenessProbe"); probeNode != nil {
		errors = append(errors, validateProbe(probeNode, "containers.livenessProbe")...)
	}

	// resources (обязательное)
	_, resourcesNode := getMap(node, "resources")
	if resourcesNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containers.resources",
			Message: "is required",
		})
	} else if resourcesNode.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    resourcesNode.Line,
			Field:   "containers.resources",
			Message: "must be mapping",
		})
	} else {
		errors = append(errors, validateResources(resourcesNode)...)
	}

	return errors
}

func validateContainerPort(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "port",
			Message: "must be mapping",
		})
		return errors
	}

	// containerPort (обязательное)
	_, portNode := getMap(node, "containerPort")
	if portNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containers.ports.containerPort",
			Message: "is required",
		})
	} else if portNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    portNode.Line,
			Field:   "containerPort",
			Message: "must be int",
		})
	} else {
		port, err := strconv.Atoi(portNode.Value)
		if err != nil {
			errors = append(errors, ValidationError{
				Line:    portNode.Line,
				Field:   "containerPort",
				Message: "must be int",
			})
		} else if port < portMin || port > portMax {
			errors = append(errors, ValidationError{
				Line:    portNode.Line,
				Field:   "containerPort",
				Message: "value out of range",
			})
		}
	}

	// protocol (необязательное)
	if _, protoNode := getMap(node, "protocol"); protoNode != nil {
		if protoNode.Kind != yaml.ScalarNode {
			errors = append(errors, ValidationError{
				Line:    protoNode.Line,
				Field:   "protocol",
				Message: "must be string",
			})
		} else {
			up := strings.ToUpper(protoNode.Value)
			if up != "TCP" && up != "UDP" {
				errors = append(errors, ValidationError{
					Line:    protoNode.Line,
					Field:   "protocol",
					Message: fmt.Sprintf("has unsupported value '%s'", protoNode.Value),
				})
			}
		}
	}

	return errors
}

func validateProbe(node *yaml.Node, field string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   field,
			Message: "must be mapping",
		})
		return errors
	}

	// httpGet (обязательное)
	_, httpGetNode := getMap(node, "httpGet")
	if httpGetNode == nil {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   field + ".httpGet",
			Message: "is required",
		})
		return errors
	}

	if httpGetNode.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    httpGetNode.Line,
			Field:   field + ".httpGet",
			Message: "must be mapping",
		})
		return errors
	}

	// path (обязательное)
	_, pathNode := getMap(httpGetNode, "path")
	if pathNode == nil {
		errors = append(errors, ValidationError{
			Line:    httpGetNode.Line,
			Field:   field + ".httpGet.path",
			Message: "is required",
		})
	} else if pathNode.Kind != yaml.ScalarNode {
		errors = append(errors, ValidationError{
			Line:    pathNode.Line,
			Field:   field + ".httpGet.path",
			Message: "must be string",
		})
	} else if !strings.HasPrefix(pathNode.Value, "/") {
		errors = append(errors, ValidationError{
			Line:    pathNode.Line,
			Field:   field + ".httpGet.path",
			Message: fmt.Sprintf("has invalid format '%s'", pathNode.Value),
		})
	}

	// port (обязательное)
	_, portNode := getMap(httpGetNode, "port")
	if portNode == nil {
		errors = append(errors, ValidationError{
			Line:    httpGetNode.Line,
			Field:   field + ".httpGet.port",
			Message: "is required",
		})
	} else if portNode.Kind != yaml.ScalarNode || portNode.Tag != "!!int" {
		errors = append(errors, ValidationError{
			Line:    portNode.Line,
			Field:   "port",
			Message: "must be int",
		})
	} else {
		port, err := strconv.Atoi(portNode.Value)
		if err != nil {
			errors = append(errors, ValidationError{
				Line:    portNode.Line,
				Field:   "port",
				Message: "must be int",
			})
		} else if port < portMin || port > portMax {
			errors = append(errors, ValidationError{
				Line:    portNode.Line,
				Field:   "port",
				Message: "value out of range",
			})
		}
	}

	return errors
}

func validateResources(node *yaml.Node) []ValidationError {
	var errors []ValidationError

	// Проверяем, что есть хотя бы одно из полей
	hasLimits := false
	hasRequests := false

	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind == yaml.ScalarNode {
			if keyNode.Value == "limits" {
				hasLimits = true
			} else if keyNode.Value == "requests" {
				hasRequests = true
			}
		}
	}

	if !hasLimits && !hasRequests {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   "containers.resources",
			Message: "must contain at least one of: requests, limits",
		})
	}

	// limits (необязательное)
	if _, limitsNode := getMap(node, "limits"); limitsNode != nil {
		errors = append(errors, validateResObj(limitsNode, "containers.resources.limits")...)
	}

	// requests (необязательное)
	if _, requestsNode := getMap(node, "requests"); requestsNode != nil {
		errors = append(errors, validateResObj(requestsNode, "containers.resources.requests")...)
	}

	return errors
}

func validateResObj(node *yaml.Node, field string) []ValidationError {
	var errors []ValidationError

	if node.Kind != yaml.MappingNode {
		errors = append(errors, ValidationError{
			Line:    node.Line,
			Field:   field,
			Message: "must be mapping",
		})
		return errors
	}

	// cpu (необязательное)
	if _, cpuNode := getMap(node, "cpu"); cpuNode != nil {
		if cpuNode.Kind != yaml.ScalarNode || cpuNode.Tag != "!!int" {
			errors = append(errors, ValidationError{
				Line:    cpuNode.Line,
				Field:   "cpu",
				Message: "must be int",
			})
		}
	}

	// memory (необязательное)
	if _, memNode := getMap(node, "memory"); memNode != nil {
		if memNode.Kind != yaml.ScalarNode {
			errors = append(errors, ValidationError{
				Line:    memNode.Line,
				Field:   "memory",
				Message: "must be string",
			})
		} else if !memoryRegex.MatchString(memNode.Value) {
			errors = append(errors, ValidationError{
				Line:    memNode.Line,
				Field:   "memory",
				Message: fmt.Sprintf("has invalid format '%s'", memNode.Value),
			})
		}
	}

	return errors
}
