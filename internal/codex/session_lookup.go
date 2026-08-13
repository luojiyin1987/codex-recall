package codex

import "strings"

func sessionCandidateFiles(home, selector string) ([]string, error) {
	files, err := DiscoverFiles(home)
	if err != nil {
		return nil, err
	}

	candidates := make([]string, 0)
	for _, path := range files {
		id, _, err := parseRolloutFilename(path)
		if err != nil {
			// Preserve support for fixtures, imported histories, or future file
			// shapes that do not follow the standard rollout filename format.
			candidates = append(candidates, path)
			continue
		}
		if strings.HasPrefix(id, selector) {
			candidates = append(candidates, path)
		}
	}
	return candidates, nil
}
