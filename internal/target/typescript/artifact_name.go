package typescript

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"
	"unicode"
)

const (
	maxArtifactSegmentBytes = 64
	maxArtifactPathBytes    = 240
)

type artifactPathCandidate struct {
	identity string
	base     string
}

func safeArtifactStem(source string) string {
	var output strings.Builder
	separator := false
	nonASCII := false
	previousLowerOrDigit := false
	for _, value := range source {
		if value > unicode.MaxASCII {
			nonASCII = true
			separator = output.Len() > 0
			previousLowerOrDigit = false
			continue
		}
		if value >= 'A' && value <= 'Z' {
			if previousLowerOrDigit && output.Len() > 0 {
				separator = true
			}
			if separator && output.Len() > 0 {
				output.WriteByte('-')
			}
			output.WriteByte(byte(value + ('a' - 'A')))
			separator = false
			previousLowerOrDigit = false
			continue
		}
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			if separator && output.Len() > 0 {
				output.WriteByte('-')
			}
			output.WriteRune(value)
			separator = false
			previousLowerOrDigit = true
			continue
		}
		separator = output.Len() > 0
		previousLowerOrDigit = false
	}
	stem := strings.Trim(output.String(), "-")
	unsafe := nonASCII || stem == "" || windowsReservedArtifactStem(stem)
	if !unsafe && len(stem) <= maxArtifactSegmentBytes-len(".ts") {
		return stem
	}
	prefix := strings.Trim(stem, "-")
	if prefix == "" {
		prefix = "value"
	}
	if len(prefix) > 40 {
		prefix = strings.TrimRight(prefix[:40], "-")
	}
	return prefix + "-" + shortArtifactHash(source)
}

func windowsReservedArtifactStem(stem string) bool {
	value := strings.ToLower(strings.TrimRight(stem, ". "))
	switch value {
	case "con", "prn", "aux", "nul", "clock$":
		return true
	}
	for _, prefix := range []string{"com", "lpt"} {
		if strings.HasPrefix(value, prefix) && len(value) == len(prefix)+1 && value[len(prefix)] >= '1' && value[len(prefix)] <= '9' {
			return true
		}
	}
	return false
}

func shortArtifactHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:6])
}

func allocateArtifactPaths(candidates []artifactPathCandidate, reservedPaths ...string) (map[string]string, error) {
	reserved := make(map[string]bool, len(reservedPaths))
	for _, value := range reservedPaths {
		if err := validateArtifactPath(value); err != nil {
			return nil, fmt.Errorf("reserved artifact path %q: %w", value, err)
		}
		reserved[portableArtifactPathKey(value)] = true
	}
	sorted := append([]artifactPathCandidate(nil), candidates...)
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].identity == sorted[right].identity {
			return sorted[left].base < sorted[right].base
		}
		return sorted[left].identity < sorted[right].identity
	})
	identities := make(map[string]bool, len(sorted))
	groups := make(map[string][]artifactPathCandidate, len(sorted))
	for _, candidate := range sorted {
		if candidate.identity == "" {
			return nil, fmt.Errorf("artifact path candidate has empty identity")
		}
		if identities[candidate.identity] {
			return nil, fmt.Errorf("artifact path identity %q is duplicated", candidate.identity)
		}
		identities[candidate.identity] = true
		if err := validateArtifactPath(candidate.base); err != nil {
			return nil, fmt.Errorf("artifact path for %q: %w", candidate.identity, err)
		}
		key := portableArtifactPathKey(candidate.base)
		groups[key] = append(groups[key], candidate)
	}
	result := make(map[string]string, len(sorted))
	used := make(map[string]string, len(sorted)+len(reserved))
	for key := range reserved {
		used[key] = "reserved"
	}
	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	for _, key := range groupKeys {
		group := groups[key]
		forceSuffix := len(group) > 1 || reserved[key]
		for _, candidate := range group {
			allocated := candidate.base
			if forceSuffix {
				allocated = suffixArtifactPath(candidate.base, shortArtifactHash(candidate.identity))
			}
			allocatedKey := portableArtifactPathKey(allocated)
			if previous, exists := used[allocatedKey]; exists {
				return nil, fmt.Errorf("artifact path %q for %q collides with %s", allocated, candidate.identity, previous)
			}
			used[allocatedKey] = candidate.identity
			result[candidate.identity] = allocated
		}
	}
	return result, nil
}

func suffixArtifactPath(value, suffix string) string {
	directory, file := pathpkg.Split(value)
	extension := pathpkg.Ext(file)
	stem := strings.TrimSuffix(file, extension)
	maximumStem := maxArtifactSegmentBytes - len(extension) - len(suffix) - 1
	if len(stem) > maximumStem {
		stem = strings.TrimRight(stem[:maximumStem], "-")
	}
	return directory + stem + "-" + suffix + extension
}

func portableArtifactPathKey(value string) string {
	return strings.ToLower(value)
}

func validateArtifactPath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("path is not a relative portable path")
	}
	if len(value) > maxArtifactPathBytes {
		return fmt.Errorf("path exceeds %d bytes", maxArtifactPathBytes)
	}
	if pathpkg.Clean(value) != value || pathpkg.Ext(value) != ".ts" {
		return fmt.Errorf("path must be clean and end in .ts")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > maxArtifactSegmentBytes {
			return fmt.Errorf("path contains an unsafe segment %q", segment)
		}
		for _, char := range segment {
			if char > unicode.MaxASCII || !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '.' || char == '_') {
				return fmt.Errorf("path segment %q contains a non-portable character", segment)
			}
		}
	}
	return nil
}

func operationArtifactBase(routePath, method string) string {
	segments := operationArtifactSegments(routePath)
	filename := safeArtifactStem(method) + ".ts"
	parts := append([]string{"internal", "operations"}, segments...)
	parts = append(parts, filename)
	result := pathpkg.Join(parts...)
	if len(result) <= maxArtifactPathBytes {
		return result
	}
	identity := strings.ToUpper(method) + " " + routePath
	return pathpkg.Join("internal", "operations", "route-"+shortArtifactHash(identity), filename)
}

func operationArtifactSegments(routePath string) []string {
	trimmed := strings.Trim(routePath, "/")
	if trimmed == "" {
		return []string{"root"}
	}
	parts := strings.Split(trimmed, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && len(part) > 2 {
			result = append(result, "by-"+safeArtifactStem(part[1:len(part)-1]))
			continue
		}
		result = append(result, safeArtifactStem(part))
	}
	return result
}

func relativeModuleSpecifier(fromArtifact, toArtifact string) (string, error) {
	if err := validateArtifactPath(fromArtifact); err != nil {
		return "", fmt.Errorf("from artifact: %w", err)
	}
	if err := validateArtifactPath(toArtifact); err != nil {
		return "", fmt.Errorf("to artifact: %w", err)
	}
	from := strings.Split(pathpkg.Dir(fromArtifact), "/")
	if len(from) == 1 && from[0] == "." {
		from = nil
	}
	to := strings.Split(strings.TrimSuffix(toArtifact, ".ts")+".js", "/")
	common := 0
	for common < len(from) && common < len(to)-1 && from[common] == to[common] {
		common++
	}
	parts := make([]string, 0, len(from)-common+len(to)-common)
	for index := common; index < len(from); index++ {
		parts = append(parts, "..")
	}
	parts = append(parts, to[common:]...)
	result := strings.Join(parts, "/")
	if !strings.HasPrefix(result, ".") {
		result = "./" + result
	}
	return result, nil
}
