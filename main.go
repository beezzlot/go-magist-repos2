package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type vErr struct {
	line int
	msg  string
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, "Usage: %s <path/to/file.yaml>\n", filepath.Base(os.Args[0]))
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	file := flag.Arg(0)
	base := filepath.Base(file)

	b, err := os.ReadFile(file)
	if err != nil {
		printFatalIOErr(file, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		fmt.Printf("%s: %v\n", base, err)
		os.Exit(1)
	}

	// Надёжно находим корневой mapping: либо документ с mapping внутри, либо сам mapping
	var top *yaml.Node
	switch root.Kind {
	case yaml.DocumentNode:
		if len(root.Content) > 0 && root.Content[0].Kind == yaml.MappingNode {
			top = root.Content[0]
		}
	case yaml.MappingNode:
		top = &root
	}
	if top == nil || top.Kind != yaml.MappingNode {
		fmt.Printf("%s: invalid YAML root (expected mapping)\n", base)
		os.Exit(1)
	}

	var errs []vErr
	validateTop(top, &errs)

	if len(errs) > 0 {
		for _, e := range errs {
			if e.line == 0 {
				fmt.Println(e.msg)
			} else {
				fmt.Printf("%s:%d %s\n", base, e.line, e.msg)
			}
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func printFatalIOErr(file string, err error) {
	base := filepath.Base(file)
	var pErr *fs.PathError
	if errors.As(err, &pErr) {
		fmt.Printf("%s: %v\n", base, pErr.Err)
	} else {
		fmt.Printf("%s: %v\n", base, err)
	}
	os.Exit(1)
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

func expectType(node *yaml.Node, kind yaml.Kind, field string, errs *[]vErr) bool {
	if node == nil || node.Kind != kind {
		t := map[yaml.Kind]string{yaml.ScalarNode: "scalar", yaml.MappingNode: "object", yaml.SequenceNode: "array"}[kind]
		if t == "" {
			t = "value"
		}
		*errs = append(*errs, vErr{line: nodeLine(node), msg: fmt.Sprintf("%s must be %s", field, t)})
		return false
	}
	return true
}

func nodeLine(n *yaml.Node) int {
	if n != nil && n.Line > 0 {
		return n.Line
	}
	return 0
}
func asString(n *yaml.Node) (string, bool) {
	if n != nil && n.Kind == yaml.ScalarNode {
		return n.Value, true
	}
	return "", false
}

func validateTop(top *yaml.Node, errs *[]vErr) {
	// apiVersion
	_, apiNode := getMap(top, "apiVersion")
	if apiNode == nil {
		*errs = append(*errs, vErr{msg: "apiVersion is required"})
	} else if expectType(apiNode, yaml.ScalarNode, "apiVersion", errs) && apiNode.Value != "v1" {
		*errs = append(*errs, vErr{line: apiNode.Line, msg: fmt.Sprintf("apiVersion has unsupported value '%s'", apiNode.Value)})
	}

	// kind
	_, kindNode := getMap(top, "kind")
	if kindNode == nil {
		*errs = append(*errs, vErr{msg: "kind is required"})
	} else if expectType(kindNode, yaml.ScalarNode, "kind", errs) && kindNode.Value != "Pod" {
		*errs = append(*errs, vErr{line: kindNode.Line, msg: fmt.Sprintf("kind has unsupported value '%s'", kindNode.Value)})
	}

	// metadata
	_, meta := getMap(top, "metadata")
	if meta == nil {
		*errs = append(*errs, vErr{msg: "metadata is required"})
	} else if expectType(meta, yaml.MappingNode, "metadata", errs) {
		validateObjectMeta(meta, errs)
	}

	// spec
	_, spec := getMap(top, "spec")
	if spec == nil {
		*errs = append(*errs, vErr{msg: "spec is required"})
	} else if expectType(spec, yaml.MappingNode, "spec", errs) {
		validatePodSpec(spec, errs)
	}
}

func validateObjectMeta(meta *yaml.Node, errs *[]vErr) {
	_, name := getMap(meta, "name")
	if name == nil {
		*errs = append(*errs, vErr{msg: "metadata.name is required"})
	} else if expectType(name, yaml.ScalarNode, "metadata.name", errs) {
		// тест ожидает "name is required" для пустой строки
		if strings.TrimSpace(name.Value) == "" {
			*errs = append(*errs, vErr{line: name.Line, msg: "name is required"})
		}
	}

	if _, ns := getMap(meta, "namespace"); ns != nil {
		expectType(ns, yaml.ScalarNode, "metadata.namespace", errs)
	}

	if _, labels := getMap(meta, "labels"); labels != nil {
		if expectType(labels, yaml.MappingNode, "metadata.labels", errs) {
			for i := 0; i < len(labels.Content)-1; i += 2 {
				k := labels.Content[i]
				v := labels.Content[i+1]
				if v.Kind != yaml.ScalarNode || k.Value == "" || v.Value == "" {
					*errs = append(*errs, vErr{line: v.Line, msg: "metadata.labels has invalid format ''"})
					break
				}
			}
		}
	}
}

func validatePodSpec(spec *yaml.Node, errs *[]vErr) {
	if _, osNode := getMap(spec, "os"); osNode != nil {
		switch osNode.Kind {
		case yaml.ScalarNode:
			validateOSName(osNode, errs)
		case yaml.MappingNode:
			_, name := getMap(osNode, "name")
			if name == nil {
				*errs = append(*errs, vErr{msg: "spec.os.name is required"})
			} else if expectType(name, yaml.ScalarNode, "spec.os.name", errs) {
				validateOSName(name, errs)
			}
		default:
			*errs = append(*errs, vErr{line: osNode.Line, msg: "spec.os must be object"})
		}
	}

	_, conts := getMap(spec, "containers")
	if conts == nil {
		*errs = append(*errs, vErr{msg: "spec.containers is required"})
	} else if expectType(conts, yaml.SequenceNode, "spec.containers", errs) {
		seen := map[string]struct{}{}
		for _, item := range conts.Content {
			if item.Kind != yaml.MappingNode {
				*errs = append(*errs, vErr{line: item.Line, msg: "spec.containers must be array"})
				continue
			}
			validateContainer(item, errs)
			if _, n := getMap(item, "name"); n != nil && n.Kind == yaml.ScalarNode {
				if _, ok := seen[n.Value]; ok {
					*errs = append(*errs, vErr{line: n.Line, msg: fmt.Sprintf("containers.name has invalid format '%s'", n.Value)})
				}
				seen[n.Value] = struct{}{}
			}
		}
	}
}

func validateOSName(n *yaml.Node, errs *[]vErr) {
	val := strings.ToLower(n.Value)
	if val != "linux" && val != "windows" {
		*errs = append(*errs, vErr{line: n.Line, msg: fmt.Sprintf("os has unsupported value '%s'", n.Value)})
	}
}

var (
	snakeRe  = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
	imageRe  = regexp.MustCompile(`^registry\.bigbrother\.io/[A-Za-z0-9._\-/]+:[A-Za-z0-9._\-]+$`)
	memQtyRe = regexp.MustCompile(`^[0-9]+(?:Gi|Mi|Ki)$`)
	portMin  = 1
	portMax  = 65535
)

func validateContainer(c *yaml.Node, errs *[]vErr) {
	_, name := getMap(c, "name")
	if name == nil {
		*errs = append(*errs, vErr{msg: "name is required"})
	} else if expectType(name, yaml.ScalarNode, "name", errs) {
		if strings.TrimSpace(name.Value) == "" {
			*errs = append(*errs, vErr{line: name.Line, msg: "name is required"})
		} else if !snakeRe.MatchString(name.Value) {
			*errs = append(*errs, vErr{line: name.Line, msg: fmt.Sprintf("containers.name has invalid format '%s'", name.Value)})
		}
	}

	_, image := getMap(c, "image")
	if image == nil {
		*errs = append(*errs, vErr{msg: "containers.image is required"})
	} else if expectType(image, yaml.ScalarNode, "containers.image", errs) && !imageRe.MatchString(image.Value) {
		*errs = append(*errs, vErr{line: image.Line, msg: fmt.Sprintf("containers.image has invalid format '%s'", image.Value)})
	}

	if _, ports := getMap(c, "ports"); ports != nil {
		if expectType(ports, yaml.SequenceNode, "containers.ports", errs) {
			for _, p := range ports.Content {
				if p.Kind != yaml.MappingNode {
					*errs = append(*errs, vErr{line: p.Line, msg: "containers.ports must be array"})
					continue
				}
				validateContainerPort(p, errs)
			}
		}
	}

	if _, rp := getMap(c, "readinessProbe"); rp != nil {
		validateProbe(rp, errs, "containers.readinessProbe")
	}
	if _, lp := getMap(c, "livenessProbe"); lp != nil {
		validateProbe(lp, errs, "containers.livenessProbe")
	}

	_, res := getMap(c, "resources")
	if res == nil {
		*errs = append(*errs, vErr{msg: "containers.resources is required"})
	} else if expectType(res, yaml.MappingNode, "containers.resources", errs) {
		validateResources(res, errs)
	}
}

func validateContainerPort(p *yaml.Node, errs *[]vErr) {
	_, cport := getMap(p, "containerPort")
	if cport == nil {
		*errs = append(*errs, vErr{msg: "containers.ports.containerPort is required"})
	} else if cport.Kind != yaml.ScalarNode {
		*errs = append(*errs, vErr{line: cport.Line, msg: "containerPort must be int"})
	} else if val, err := strconv.Atoi(cport.Value); err != nil {
		*errs = append(*errs, vErr{line: cport.Line, msg: "containerPort must be int"})
	} else if val < portMin || val > portMax {
		*errs = append(*errs, vErr{line: cport.Line, msg: "containerPort value out of range"})
	}

	if _, proto := getMap(p, "protocol"); proto != nil {
		if !expectType(proto, yaml.ScalarNode, "protocol", errs) {
			return
		}
		up := strings.ToUpper(proto.Value)
		if up != "TCP" && up != "UDP" {
			*errs = append(*errs, vErr{line: proto.Line, msg: fmt.Sprintf("protocol has unsupported value '%s'", proto.Value)})
		}
	}
}

func validateProbe(n *yaml.Node, errs *[]vErr, field string) {
	if !expectType(n, yaml.MappingNode, field, errs) {
		return
	}
	_, httpGet := getMap(n, "httpGet")
	if httpGet == nil {
		*errs = append(*errs, vErr{msg: field + ".httpGet is required"})
		return
	}
	if !expectType(httpGet, yaml.MappingNode, field+".httpGet", errs) {
		return
	}

	_, path := getMap(httpGet, "path")
	if path == nil {
		*errs = append(*errs, vErr{msg: field + ".httpGet.path is required"})
	} else if expectType(path, yaml.ScalarNode, field+".httpGet.path", errs) && !strings.HasPrefix(path.Value, "/") {
		*errs = append(*errs, vErr{line: path.Line, msg: fmt.Sprintf("%s has invalid format '%s'", field+".httpGet.path", path.Value)})
	}

	_, port := getMap(httpGet, "port")
	if port == nil {
		*errs = append(*errs, vErr{msg: field + ".httpGet.port is required"})
		return
	}
	if port.Kind != yaml.ScalarNode || port.Tag != "!!int" {
		*errs = append(*errs, vErr{line: port.Line, msg: "port must be int"})
		return
	}
	if val, err := strconv.Atoi(port.Value); err == nil {
		if val < portMin || val > portMax {
			*errs = append(*errs, vErr{line: port.Line, msg: "port value out of range"})
		}
	} else {
		*errs = append(*errs, vErr{line: port.Line, msg: "port must be int"})
	}
}

func validateResources(n *yaml.Node, errs *[]vErr) {
	if _, limits := getMap(n, "limits"); limits != nil {
		validateResObj(limits, "containers.resources.limits", errs)
	}
	if _, req := getMap(n, "requests"); req != nil {
		validateResObj(req, "containers.resources.requests", errs)
	}
}

func validateResObj(n *yaml.Node, field string, errs *[]vErr) {
	if !expectType(n, yaml.MappingNode, field, errs) {
		return
	}
	if _, cpu := getMap(n, "cpu"); cpu != nil {
		if cpu.Kind != yaml.ScalarNode || cpu.Tag != "!!int" {
			*errs = append(*errs, vErr{line: cpu.Line, msg: "cpu must be int"})
		}
	}
	if _, mem := getMap(n, "memory"); mem != nil {
		if s, ok := asString(mem); !ok {
			*errs = append(*errs, vErr{line: mem.Line, msg: "memory must be string"})
		} else if !memQtyRe.MatchString(s) {
			*errs = append(*errs, vErr{line: mem.Line, msg: fmt.Sprintf("memory has invalid format '%s'", s)})
		}
	}
}
