package skillsbuiltin

// SkillMeta is one markdown skill file under the skills root (no structured parse).
type SkillMeta struct {
	Name       string `json:"name"`
	SourceFile string `json:"source_file"`
	SourcePath string `json:"source_path"`
	ByteSize   int64  `json:"byte_size"`
}
