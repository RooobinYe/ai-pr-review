package review

import (
	"encoding/json"
)

// FileCategory classifies a changed file by its role in the project.
type FileCategory int

const (
	FileCategoryCode  FileCategory = iota // production source code
	FileCategoryTest                      // test files
	FileCategoryConfig                    // configuration, CI/CD, build scripts
	FileCategoryDoc                       // documentation, markdown
	FileCategoryOther                     // uncategorised
)

var categoryNames = map[FileCategory]string{
	FileCategoryCode:   "code",
	FileCategoryTest:   "test",
	FileCategoryConfig: "config",
	FileCategoryDoc:    "doc",
	FileCategoryOther:  "other",
}

func (c FileCategory) String() string {
	if s, ok := categoryNames[c]; ok {
		return s
	}
	return "other"
}

func (c FileCategory) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *FileCategory) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for k, v := range categoryNames {
		if v == s {
			*c = k
			return nil
		}
	}
	*c = FileCategoryOther
	return nil
}
