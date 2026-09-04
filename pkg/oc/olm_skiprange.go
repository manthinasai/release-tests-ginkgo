package oc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

// FetchOlmSkipRange fetches OLM skipRange data from the openshift-pipelines-operator-rh
// package manifest, returning a map of channel name → skipRange string.
func FetchOlmSkipRange() (map[string]string, error) {
	operatorEnv := os.Getenv("OPERATOR_ENVIRONMENT")
	catalogSource := os.Getenv("CATALOG_SOURCE")
	var catalog string

	switch {
	case operatorEnv != "":
		switch operatorEnv {
		case "pre-stage", "stage":
			catalog = "custom-operators"
		case "prod":
			catalog = "redhat-operators"
		default:
			catalog = "redhat-operators"
		}
	case catalogSource != "":
		catalog = catalogSource
	default:
		catalog = "redhat-operators"
	}

	log.Printf("FetchOlmSkipRange: using catalog %q", catalog) //nolint:gosec // G706

	packageManifestsJSON := cmd.MustSucceed("oc", "get", "packagemanifest", "-n", "openshift-marketplace",
		"--selector=catalog="+catalog, "-o", "json").Stdout()

	type channel struct {
		Name           string `json:"name"`
		CurrentCSVDesc struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"currentCSVDesc"`
	}
	type packageManifest struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Channels []channel `json:"channels"`
		} `json:"status"`
	}
	type packageManifestList struct {
		Items []packageManifest `json:"items"`
	}

	var list packageManifestList
	if err := json.Unmarshal([]byte(packageManifestsJSON), &list); err != nil {
		return nil, fmt.Errorf("failed to parse package manifest list JSON: %w", err)
	}

	var target *packageManifest
	for i := range list.Items {
		if list.Items[i].Metadata.Name == "openshift-pipelines-operator-rh" {
			target = &list.Items[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("package 'openshift-pipelines-operator-rh' not found in catalog %q", catalog)
	}

	result := make(map[string]string)
	for _, ch := range target.Status.Channels {
		if sr, ok := ch.CurrentCSVDesc.Annotations["olm.skipRange"]; ok && sr != "" {
			result[ch.Name] = sr
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no valid OLM skip ranges found in catalog %q", catalog)
	}

	log.Printf("FetchOlmSkipRange: found %d channels with skipRange", len(result))
	return result, nil
}

// extractMajorMinor extracts the "X.Y" portion from a version string like "1.19.2".
func extractMajorMinor(version string) string {
	re := regexp.MustCompile(`^(\d+\.\d+)\.?\d*`)
	if m := re.FindStringSubmatch(version); len(m) >= 2 {
		return m[1]
	}
	return version
}

func skipRangeContainsVersion(skipRange, version string) bool {
	return strings.Contains(skipRange, version)
}

// isValidOspVersionPatchUpdate returns true when postSkipRange represents a valid patch
// increment of preSkipRange for the current OSP_VERSION (lower bound unchanged, upper
// bound increased by one patch).
func isValidOspVersionPatchUpdate(preSkipRange, postSkipRange string) bool {
	ospVersion := os.Getenv("OSP_VERSION")
	if ospVersion == "" {
		return false
	}
	if !skipRangeContainsVersion(postSkipRange, ospVersion) {
		return false
	}

	re := regexp.MustCompile(`>=(\d+\.\d+\.\d+)\s*<(\d+\.\d+\.\d+)`)
	pre := re.FindStringSubmatch(preSkipRange)
	post := re.FindStringSubmatch(postSkipRange)
	if len(pre) != 3 || len(post) != 3 {
		return false
	}
	if pre[1] != post[1] { // lower bound must not change
		return false
	}

	patchRe := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	preU := patchRe.FindStringSubmatch(pre[2])
	postU := patchRe.FindStringSubmatch(post[2])
	if len(preU) != 4 || len(postU) != 4 {
		return false
	}

	var preP, postP int
	if _, err := fmt.Sscanf(preU[3], "%d", &preP); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(postU[3], "%d", &postP); err != nil {
		return false
	}
	return postP > preP
}

// ValidateOlmSkipRange verifies that OSP_VERSION appears in the correct channel's skipRange.
// Maps to Gauge step "Validate OSP Version in OlmSkipRange".
func (oc *OC) ValidateOlmSkipRange() {
	skipRangeMap, err := FetchOlmSkipRange()
	if err != nil {
		Fail(fmt.Sprintf("failed to fetch OLM skip range: %v", err))
	}

	ospVersion := os.Getenv("OSP_VERSION")
	if ospVersion == "" {
		Skip("OSP_VERSION not set -- skipping OLM skip range validation")
	}

	found := false
	if ospVersion == "5.0.5" {
		// Nightly build: only check that skipRange contains the version string.
		for channel, skipRange := range skipRangeMap {
			if channel == "latest" {
				continue
			}
			if strings.Contains(skipRange, ospVersion) {
				log.Printf("OSP_VERSION %q found in skipRange of channel %q: %q", ospVersion, channel, skipRange) //nolint:gosec // G706
				found = true
				break
			}
		}
	} else {
		ospMajorMinor := extractMajorMinor(ospVersion)
		for channel, skipRange := range skipRangeMap {
			if channel == "latest" {
				continue
			}
			if strings.Contains(channel, ospMajorMinor) && strings.Contains(skipRange, ospVersion) {
				log.Printf("OSP_VERSION %q found in channel %q with skipRange %q", ospVersion, channel, skipRange) //nolint:gosec // G706
				found = true
				break
			}
		}
	}

	if !found {
		for ch, sr := range skipRangeMap {
			if ch != "latest" {
				log.Printf("  channel %q: %q", ch, sr)
			}
		}
		Fail(fmt.Sprintf("OSP_VERSION %q not found in any non-latest channel's skipRange", ospVersion))
	}
}

// ValidateChannelSkipRangeBounds validates that every channel's skipRange lower bound
// refers to the preceding minor version and the upper bound matches the channel's minor version.
// Maps to Gauge step "Validate all channels have valid skipRange bounds".
func (oc *OC) ValidateChannelSkipRangeBounds() {
	skipRangeMap, err := FetchOlmSkipRange()
	if err != nil {
		Fail(fmt.Sprintf("failed to fetch OLM skip range: %v", err))
	}

	srPattern := regexp.MustCompile(`>=(\d+\.\d+\.\d+)\s*<(\d+\.\d+\.\d+)`)
	chVersionPattern := regexp.MustCompile(`pipelines-(\d+)\.(\d+)`)
	var errs []string

	for channel, skipRange := range skipRangeMap {
		if channel == "latest" {
			continue
		}

		chM := chVersionPattern.FindStringSubmatch(channel)
		if len(chM) != 3 {
			log.Printf("Channel %q: cannot extract version from name, skipping bounds check", channel)
			continue
		}
		var chMajor, chMinor int
		_, _ = fmt.Sscanf(chM[1], "%d", &chMajor)
		_, _ = fmt.Sscanf(chM[2], "%d", &chMinor)
		channelVersion := fmt.Sprintf("%d.%d", chMajor, chMinor)

		srM := srPattern.FindStringSubmatch(skipRange)
		if len(srM) != 3 {
			errs = append(errs, fmt.Sprintf("channel %q has invalid skipRange format: %q", channel, skipRange))
			continue
		}
		lowerBound := srM[1]
		upperBound := srM[2]

		upperMajorMinor := extractMajorMinor(upperBound)

		lowerParts := strings.SplitN(extractMajorMinor(lowerBound), ".", 2)
		var lowerMajor int
		_, _ = fmt.Sscanf(lowerParts[0], "%d", &lowerMajor)
		if lowerMajor != chMajor {
			errs = append(errs, fmt.Sprintf("channel %q lower bound %q has unexpected major version", channel, lowerBound))
		}

		if upperMajorMinor != channelVersion {
			errs = append(errs, fmt.Sprintf("channel %q upper bound %q (major.minor %q) doesn't match channel version %q", channel, upperBound, upperMajorMinor, channelVersion))
		} else {
			log.Printf("Channel %q: bounds OK (lower %q, upper %q)", channel, lowerBound, upperBound)
		}
	}

	if len(errs) > 0 {
		Fail(fmt.Sprintf("channel skipRange bounds validation failed:\n%s", strings.Join(errs, "\n")))
	}
}

// GetOlmSkipRange fetches the current OLM skipRange map and persists it to fileName
// under the upgradeType key (e.g. "pre-upgrade-olm-skip-range").
// Maps to Gauge step "Get olm-skip-range <upgradeType> and save to field <fieldName> in file <fileName>".
func (oc *OC) GetOlmSkipRange(upgradeType, _ /* fieldName */ string, fileName string) {
	skipRangeMap, err := FetchOlmSkipRange()
	if err != nil {
		Fail(fmt.Sprintf("failed to fetch OLM skip range: %v", err))
	}

	filePath := config.Path(fileName)

	file, err := os.OpenFile(filePath, os.O_RDWR, 0644) //nolint:gosec
	if err != nil {
		Fail(fmt.Sprintf("failed to open %s: %v", fileName, err))
	}
	defer file.Close() //nolint:errcheck

	var existing map[string]interface{}
	if err := json.NewDecoder(file).Decode(&existing); err != nil {
		Fail(fmt.Sprintf("failed to decode JSON from %s: %v", fileName, err))
	}

	fieldKey := fmt.Sprintf("%s-olm-skip-range", upgradeType)
	existing[fieldKey] = skipRangeMap
	log.Printf("GetOlmSkipRange: saving %d channels under key %q to %s", len(skipRangeMap), fieldKey, fileName)

	if _, err := file.Seek(0, 0); err != nil {
		Fail(fmt.Sprintf("failed to seek %s: %v", fileName, err))
	}
	if err := file.Truncate(0); err != nil {
		Fail(fmt.Sprintf("failed to truncate %s: %v", fileName, err))
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(existing); err != nil {
		Fail(fmt.Sprintf("failed to write JSON to %s: %v", fileName, err))
	}
}

// ValidateOlmSkipRangeDiff validates that between pre- and post-upgrade only the channel
// matching OSP_VERSION had its skipRange upper bound incremented.
// Maps to Gauge step "Validate skipRange diff between fields <pre> and <post> in file <fileName>".
func (oc *OC) ValidateOlmSkipRangeDiff(fileName, preUpgradeField, postUpgradeField string) {
	filePath := config.Path(fileName)
	file, err := os.Open(filePath) //nolint:gosec
	if err != nil {
		Fail(fmt.Sprintf("failed to open %s: %v", fileName, err))
	}
	defer file.Close() //nolint:errcheck

	var data map[string]interface{}
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		Fail(fmt.Sprintf("failed to decode JSON from %s: %v", fileName, err))
	}

	preRaw, preOK := data[preUpgradeField]
	postRaw, postOK := data[postUpgradeField]
	if !preOK || !postOK {
		Fail(fmt.Sprintf("skiprange file missing fields: pre exists=%v, post exists=%v", preOK, postOK))
	}

	preMap, ok1 := preRaw.(map[string]interface{})
	postMap, ok2 := postRaw.(map[string]interface{})
	if !ok1 || !ok2 {
		Fail("skiprange data fields are not in expected map format")
	}

	ospVersion := os.Getenv("OSP_VERSION")
	ospMajorMinor := ""
	if ospVersion != "" {
		ospMajorMinor = extractMajorMinor(ospVersion)
	}

	srPattern := regexp.MustCompile(`>=(\d+\.\d+\.\d+)\s*<(\d+\.\d+\.\d+)`)
	var errs []string

	for channel, preV := range preMap {
		if channel == "latest" {
			continue
		}
		preSkipRange, _ := preV.(string)
		postV, exists := postMap[channel]
		if !exists {
			errs = append(errs, fmt.Sprintf("channel %q missing in post-upgrade data", channel))
			continue
		}
		postSkipRange, _ := postV.(string)

		channelMatchesOSP := ospVersion != "" && strings.Contains(channel, ospMajorMinor)

		if preSkipRange == postSkipRange {
			if channelMatchesOSP {
				errs = append(errs, fmt.Sprintf("channel %q (matches OSP_VERSION %s) skipRange unchanged; expected upper bound update", channel, ospVersion))
			} else {
				log.Printf("Channel %q: unchanged (expected)", channel)
			}
			continue
		}

		log.Printf("Channel %q: %q → %q", channel, preSkipRange, postSkipRange)

		pre := srPattern.FindStringSubmatch(preSkipRange)
		post := srPattern.FindStringSubmatch(postSkipRange)
		if len(pre) != 3 || len(post) != 3 {
			errs = append(errs, fmt.Sprintf("channel %q has invalid skipRange format", channel))
			continue
		}

		if channelMatchesOSP {
			if pre[1] != post[1] {
				errs = append(errs, fmt.Sprintf("channel %q lower bound changed from %q to %q (must be unchanged)", channel, pre[1], post[1]))
				continue
			}
			if isValidOspVersionPatchUpdate(preSkipRange, postSkipRange) {
				log.Printf("Channel %q: valid patch update for OSP_VERSION %q", channel, ospVersion) //nolint:gosec // G706
			} else {
				errs = append(errs, fmt.Sprintf("channel %q has invalid patch update: %q → %q", channel, preSkipRange, postSkipRange))
			}
		} else {
			errs = append(errs, fmt.Sprintf("channel %q skipRange changed unexpectedly: %q → %q", channel, preSkipRange, postSkipRange))
		}
	}

	// Check for entirely new channels in post-upgrade.
	for postChannel, postV := range postMap {
		if postChannel == "latest" {
			continue
		}
		if _, existed := preMap[postChannel]; existed {
			continue
		}
		postSkipRange, _ := postV.(string)
		channelMatchesOSP := ospVersion != "" && strings.Contains(postChannel, ospMajorMinor)
		if channelMatchesOSP {
			post := srPattern.FindStringSubmatch(postSkipRange)
			if len(post) != 3 {
				errs = append(errs, fmt.Sprintf("new channel %q has invalid skipRange format: %q", postChannel, postSkipRange))
				continue
			}
			if extractMajorMinor(post[2]) != ospMajorMinor {
				errs = append(errs, fmt.Sprintf("new channel %q upper bound %q doesn't match OSP_VERSION %q", postChannel, post[2], ospVersion))
				continue
			}
			if !skipRangeContainsVersion(postSkipRange, ospVersion) {
				errs = append(errs, fmt.Sprintf("new channel %q skipRange %q does not contain OSP_VERSION %q", postChannel, postSkipRange, ospVersion))
				continue
			}
			log.Printf("New channel %q: valid for OSP_VERSION %q", postChannel, ospVersion) //nolint:gosec // G706
		} else {
			log.Printf("New channel %q found but does not match OSP_VERSION %q", postChannel, ospVersion) //nolint:gosec // G706
		}
	}

	if len(errs) > 0 {
		Fail(fmt.Sprintf("OLM skip range diff validation failed:\n%s", strings.Join(errs, "\n")))
	}
	log.Printf("OLM skip range diff validation passed")
}
