// Package catalog holds the user's software list: categories, tags and the
// apps with their install source. The list is one JSON file, which is also
// the import and export format.
package catalog

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"time"

	"appshelf/internal/jsonfile"
)

// SchemaVersion is the version of the file format this build reads and
// writes.
const SchemaVersion = 1

// Catalog is the whole software list.
type Catalog struct {
	SchemaVersion int        `json:"schemaVersion"`
	Categories    []Category `json:"categories"`
	Tags          []Tag      `json:"tags"`
	Apps          []App      `json:"apps"`
}

// Category groups apps; an app belongs to at most one.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Tag labels apps; an app can have several.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// App is one piece of software in the list.
type App struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	CategoryID string   `json:"categoryId,omitempty"`
	TagIDs     []string `json:"tagIds,omitempty"`
	Source     Source   `json:"source"`
	Detect     *Detect  `json:"detect,omitempty"`
	// Note is the user's own note on the app, in Markdown.
	Note string `json:"note,omitempty"`
}

// SourceType names where an app is installed from.
type SourceType string

const (
	Winget SourceType = "winget"
	Scoop  SourceType = "scoop"
	GitHub SourceType = "github"
)

// SourceTypes lists the source types in the order the interface shows them.
var SourceTypes = []SourceType{Winget, Scoop, GitHub}

// Source says where and how an app is installed. Which fields apply depends
// on Type.
type Source struct {
	Type SourceType `json:"type"`

	// winget
	ID string `json:"id,omitempty"`

	// scoop
	Bucket    string `json:"bucket,omitempty"`
	Name      string `json:"name,omitempty"`
	BucketURL string `json:"bucketUrl,omitempty"`

	// github
	Repo          string `json:"repo,omitempty"`
	AssetPattern  string `json:"assetPattern,omitempty"`
	InstallerArgs string `json:"installerArgs,omitempty"`
}

// Detect tells how to recognize an app installed outside its source: by the
// DisplayName of an entry under the registry's Uninstall keys (a regular
// expression, matched ignoring case) or by an executable that exists. Either
// one matching counts as installed.
type Detect struct {
	DisplayName string `json:"displayName,omitempty"`
	ExePath     string `json:"exePath,omitempty"`
}

// New returns an empty catalog.
func New() *Catalog {
	return &Catalog{SchemaVersion: SchemaVersion, Categories: []Category{}, Tags: []Tag{}, Apps: []App{}}
}

// Key identifies the source of an app: two apps with the same key would
// install the same thing. Keys compare without case.
func (s Source) Key() string {
	switch s.Type {
	case Winget:
		return "winget:" + strings.ToLower(s.ID)
	case Scoop:
		return "scoop:" + strings.ToLower(s.Bucket) + "/" + strings.ToLower(s.Name)
	case GitHub:
		return "github:" + strings.ToLower(s.Repo)
	}
	return string(s.Type) + ":"
}

// Display is the exact identity of the source, as the interface shows it.
func (s Source) Display() string {
	switch s.Type {
	case Winget:
		return s.ID
	case Scoop:
		return s.Bucket + "/" + s.Name
	case GitHub:
		return s.Repo
	}
	return ""
}

// ValidationError tells what is wrong with a catalog or an app. Field names
// the field of an app it concerns, as "source.id", when there is one.
type ValidationError struct {
	Where   string // the app or the part of the file, for messages
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Where == "" {
		return e.Message
	}
	return e.Where + "：" + e.Message
}

var (
	idPattern        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	wingetIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`)
	scoopNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,127}$`)
	bucketPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	repoPattern      = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
)

const maxNameLen = 200

// MaxNoteLen is how long a note can be, in characters.
const MaxNoteLen = 20000

// Parse reads a catalog from JSON and validates all of it.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	// Editors such as Notepad may start the file with a byte order mark.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, &ValidationError{Message: "不是有效的清单文件：" + err.Error()}
	}
	if c.SchemaVersion == 0 {
		return nil, &ValidationError{Message: "缺少 schemaVersion，不是 AppShelf 清单文件"}
	}
	if c.SchemaVersion > SchemaVersion {
		return nil, &ValidationError{Message: fmt.Sprintf("清单版本 %d 比当前程序支持的版本 %d 新，请先更新 AppShelf", c.SchemaVersion, SchemaVersion)}
	}
	if c.Categories == nil {
		c.Categories = []Category{}
	}
	if c.Tags == nil {
		c.Tags = []Tag{}
	}
	if c.Apps == nil {
		c.Apps = []App{}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Load reads the catalog at path; a missing file is an empty catalog.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Save validates the catalog and writes it to path.
func (c *Catalog) Save(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return jsonfile.WriteAtomic(path, c)
}

// Marshal returns the catalog as the JSON of its file.
func (c *Catalog) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Validate checks the whole catalog: unique IDs, names and sources,
// references that exist, and every app's fields.
func (c *Catalog) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return &ValidationError{Message: fmt.Sprintf("不支持的清单版本 %d", c.SchemaVersion)}
	}
	cats := map[string]bool{}
	catNames := map[string]bool{}
	for i, cat := range c.Categories {
		where := fmt.Sprintf("第 %d 个分类", i+1)
		if !idPattern.MatchString(cat.ID) {
			return &ValidationError{Where: where, Message: "ID 格式不正确"}
		}
		if cats[cat.ID] {
			return &ValidationError{Where: where, Message: "ID 重复：" + cat.ID}
		}
		cats[cat.ID] = true
		if err := checkName(cat.Name); err != nil {
			return &ValidationError{Where: where, Message: err.Error()}
		}
		n := foldName(cat.Name)
		if catNames[n] {
			return &ValidationError{Where: where, Message: "分类名称重复：" + cat.Name}
		}
		catNames[n] = true
	}
	tags := map[string]bool{}
	tagNames := map[string]bool{}
	for i, tag := range c.Tags {
		where := fmt.Sprintf("第 %d 个标签", i+1)
		if !idPattern.MatchString(tag.ID) {
			return &ValidationError{Where: where, Message: "ID 格式不正确"}
		}
		if tags[tag.ID] {
			return &ValidationError{Where: where, Message: "ID 重复：" + tag.ID}
		}
		tags[tag.ID] = true
		if err := checkName(tag.Name); err != nil {
			return &ValidationError{Where: where, Message: err.Error()}
		}
		n := foldName(tag.Name)
		if tagNames[n] {
			return &ValidationError{Where: where, Message: "标签名称重复：" + tag.Name}
		}
		tagNames[n] = true
	}
	ids := map[string]bool{}
	keys := map[string]string{}
	for i, app := range c.Apps {
		where := fmt.Sprintf("第 %d 个软件", i+1)
		if strings.TrimSpace(app.Name) != "" {
			where = "软件「" + app.Name + "」"
		}
		if !idPattern.MatchString(app.ID) {
			return &ValidationError{Where: where, Field: "id", Message: "ID 格式不正确"}
		}
		if ids[app.ID] {
			return &ValidationError{Where: where, Field: "id", Message: "ID 重复：" + app.ID}
		}
		ids[app.ID] = true
		if err := ValidateApp(app); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				ve.Where = where
			}
			return err
		}
		if app.CategoryID != "" && !cats[app.CategoryID] {
			return &ValidationError{Where: where, Field: "categoryId", Message: "分类不存在：" + app.CategoryID}
		}
		seen := map[string]bool{}
		for _, t := range app.TagIDs {
			if !tags[t] {
				return &ValidationError{Where: where, Field: "tagIds", Message: "标签不存在：" + t}
			}
			if seen[t] {
				return &ValidationError{Where: where, Field: "tagIds", Message: "标签重复：" + t}
			}
			seen[t] = true
		}
		key := app.Source.Key()
		if other, ok := keys[key]; ok {
			return &ValidationError{Where: where, Field: "source", Message: "与软件「" + other + "」的来源相同：" + app.Source.Display()}
		}
		keys[key] = app.Name
	}
	return nil
}

// ValidateApp checks one app's own fields, without the references to
// categories and tags or uniqueness in a catalog.
func ValidateApp(app App) error {
	if err := checkName(app.Name); err != nil {
		return &ValidationError{Field: "name", Message: err.Error()}
	}
	s := app.Source
	switch s.Type {
	case Winget:
		if !wingetIDPattern.MatchString(s.ID) {
			return &ValidationError{Field: "source.id", Message: "winget ID 格式不正确：" + s.ID}
		}
		if s.Bucket != "" || s.Name != "" || s.BucketURL != "" || s.Repo != "" || s.AssetPattern != "" || s.InstallerArgs != "" {
			return &ValidationError{Field: "source", Message: "winget 来源只能填写 id"}
		}
	case Scoop:
		if !bucketPattern.MatchString(s.Bucket) {
			return &ValidationError{Field: "source.bucket", Message: "scoop bucket 格式不正确：" + s.Bucket}
		}
		if !scoopNamePattern.MatchString(s.Name) {
			return &ValidationError{Field: "source.name", Message: "scoop 软件名格式不正确：" + s.Name}
		}
		if s.BucketURL != "" && (!strings.HasPrefix(s.BucketURL, "https://") || strings.ContainsAny(s.BucketURL, " \t\r\n\"'`")) {
			return &ValidationError{Field: "source.bucketUrl", Message: "bucket 地址必须是 https:// 开头的网址"}
		}
		if s.ID != "" || s.Repo != "" || s.AssetPattern != "" || s.InstallerArgs != "" {
			return &ValidationError{Field: "source", Message: "scoop 来源只能填写 bucket、name 和 bucketUrl"}
		}
	case GitHub:
		if !repoPattern.MatchString(s.Repo) {
			return &ValidationError{Field: "source.repo", Message: "GitHub 仓库应写成 owner/name：" + s.Repo}
		}
		if s.AssetPattern != "" {
			if _, err := regexp.Compile(s.AssetPattern); err != nil {
				return &ValidationError{Field: "source.assetPattern", Message: "安装包匹配规则不是有效的正则表达式"}
			}
		}
		if len(s.InstallerArgs) > 1024 || strings.ContainsAny(s.InstallerArgs, "\r\n\x00") {
			return &ValidationError{Field: "source.installerArgs", Message: "安装参数不能换行，且不超过 1024 个字符"}
		}
		if s.ID != "" || s.Bucket != "" || s.Name != "" || s.BucketURL != "" {
			return &ValidationError{Field: "source", Message: "GitHub 来源只能填写 repo、assetPattern 和 installerArgs"}
		}
		if app.Detect == nil || (app.Detect.DisplayName == "" && app.Detect.ExePath == "") {
			return &ValidationError{Field: "detect", Message: "GitHub 来源必须填写检测规则"}
		}
	default:
		return &ValidationError{Field: "source.type", Message: "未知的来源类型：" + string(s.Type)}
	}
	if len([]rune(app.Note)) > MaxNoteLen {
		return &ValidationError{Field: "note", Message: fmt.Sprintf("备注不能超过 %d 个字", MaxNoteLen)}
	}
	if strings.ContainsRune(app.Note, 0) {
		return &ValidationError{Field: "note", Message: "备注含有无效字符"}
	}
	if d := app.Detect; d != nil {
		if d.DisplayName == "" && d.ExePath == "" {
			return &ValidationError{Field: "detect", Message: "检测规则至少要填写一项"}
		}
		if d.DisplayName != "" {
			if _, err := regexp.Compile(d.DisplayName); err != nil {
				return &ValidationError{Field: "detect.displayName", Message: "注册表名称规则不是有效的正则表达式"}
			}
		}
		if len(d.ExePath) > 1024 || strings.ContainsAny(d.ExePath, "\r\n\x00\"") {
			return &ValidationError{Field: "detect.exePath", Message: "可执行文件路径格式不正确"}
		}
	}
	return nil
}

func checkName(name string) error {
	t := strings.TrimSpace(name)
	switch {
	case t == "":
		return errors.New("名称不能为空")
	case t != name:
		return errors.New("名称前后不能有空格")
	case len([]rune(t)) > maxNameLen:
		return fmt.Errorf("名称不能超过 %d 个字", maxNameLen)
	case strings.ContainsAny(t, "\r\n\x00"):
		return errors.New("名称不能换行")
	}
	return nil
}

func foldName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// NewID returns a new random ID with a prefix, as "a_7f3k2m9q".
func NewID(prefix string) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported systems; fall back to time.
		n := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(n >> (i * 8))
		}
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return prefix + "_" + string(b)
}
