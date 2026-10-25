package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Layer 是一层配置文件。layers 按优先级从高到低排。
//
// Project 标出「项目级 .council/config.toml」：它唯一需要区别对待的地方是
// data_dir —— 项目层写它也一律忽略（任务书 §1），因为数据目录跟着项目走
// 会让同一个项目在两台机器、或同一个人的两个项目之间裂成两个库。
type Layer struct {
	Path    string
	Project bool
}

// MergedConfig 是 LoadMerged 的结果。
type MergedConfig struct {
	// Config 是合并、填完缺省值并通过 Validate 的配置。
	Config *Config
	// Layers 是实际参与合并的文件，顺序与输入一致（高优先级在前）。
	Layers []string
	// RulesFileBase 是 rules_file 的定义层所在目录：cfg.RulesFile 是相对路径时
	// 要相对它解析。写它的那份配置才决定它相对谁，不能一律拿最高优先层。
	RulesFileBase string
	// Notices 是只给 Verbose 档的提示（目前只有「项目层写了 data_dir」一种）。
	// 它们不是错误，调用方可以选择不打印。
	Notices []string
}

// ResolveConfigLayers 按任务书 §1 的顺序求要合并的配置层：
//
//  1. -config（flagValue 非空）
//  2. COUNCIL_CONFIG
//  3. 从 cwd 向上到仓库根，最近的一份 .council/config.toml（只取一份）
//  4. ~/.council/config.toml
//
// 1 / 2 是「用户点名的一份文件」，按整份替换处理：只读它，不再叠项目层与
// 全局层。这是既有语义（--config 指的是「用这一份」）。若还往下合并，用户
// 拿一份临时配置调试时会被自己家目录里的真实配置悄悄污染，现有测试也会
// 开始依赖跑测试那台机器的 ~/.council/config.toml。
//
// 3 / 4 才合并，返回顺序是「项目层在前、全局层在后」，与优先级一致。
// 两个文件都不存在时返回一条全局位置的占位层：调用方靠它的 ENOENT 判断
// 「一份配置都没有」，第一次运行与只读看板的回落都建立在这个判断上。
func ResolveConfigLayers(flagValue string) ([]Layer, error) {
	if flagValue != "" {
		return []Layer{{Path: flagValue}}, nil
	}
	if env := os.Getenv(EnvConfigPath); env != "" {
		return []Layer{{Path: env}}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve default config path: user home dir: %w", err)
	}
	globalPath := filepath.Join(home, ".council", "config.toml")

	projectPath := ""
	hasProject := false
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		projectPath, hasProject = firstProjectConfig(projectDirs(cwd))
	}

	layers := make([]Layer, 0, 2)
	// cwd 恰好是用户主目录（或仓库根就是主目录）时，项目层与全局层会指向
	// 同一个文件。这时按全局层处理一份就够：既不会重复合并，也不会因为
	// 它是「项目层」而把唯一那份 data_dir 也忽略掉。
	if hasProject && projectPath != globalPath {
		layers = append(layers, Layer{Path: projectPath, Project: true})
	}

	_, statErr := os.Stat(globalPath)
	globalExists := statErr == nil || !errors.Is(statErr, fs.ErrNotExist)
	if globalExists || len(layers) == 0 {
		layers = append(layers, Layer{Path: globalPath})
	}
	return layers, nil
}

// LoadMerged 按优先级从高到低合并 layers，填缺省值、跑 Validate。
//
// 合并规则（任务书 §2）：
//   - [providers.*] / [agents.*] / [teams.*]：同名条目由高优先级层整条覆盖，
//     没提到的条目从低优先级层继承；
//   - [summary] 与 rules_file / store_thought：高优先级层写了才覆盖，没写就继承；
//   - data_dir：只认非项目层（-config / COUNCIL_CONFIG / 全局层），项目层里
//     写了也忽略，并留一条 Notice；
//   - 合并完成后必须通过 Validate。合并了多层时，校验错误逐条标上
//     「来自哪一份文件」——合并之后光看错误路径已经推不出该改哪个文件了；
//     只有一层时不标，错误文案与单文件 Load/Validate 逐字相同（repl 的
//     /add 写后校验与 /config reload 都依赖那份措辞）。
//
// Load 保持原有的单文件语义，不经过这里。repl 的 /add 守卫与 /config reload
// 走的是本函数：它们先用 ResolveConfigLayers 求出「这次该读哪几层」，单文件
// （用户显式 -config / COUNCIL_CONFIG）时就是一层的 LoadMerged，与 Load 的
// 加载结果相同，只是多跑一次 Validate。
func LoadMerged(layers []Layer) (MergedConfig, error) {
	if len(layers) == 0 {
		return MergedConfig{}, errors.New("没有可合并的配置文件")
	}

	// 结构体而不是直接就地合并：Metadata 要一直留到合并阶段，用来判断
	// 某一层到底写没写某个键。「没写」与「写了个零值」必须分开处理，
	// 否则全局的 store_thought = false 会被项目层省缺的 true 悄悄翻回去。
	type decodedLayer struct {
		layer Layer
		cfg   Config
		md    toml.MetaData
	}
	decoded := make([]decodedLayer, 0, len(layers))
	for _, layer := range layers {
		if layer.Path == "" {
			return MergedConfig{}, errors.New("配置层路径为空")
		}
		var cfg Config
		md, err := decodeConfigFile(layer.Path, &cfg)
		if err != nil {
			return MergedConfig{}, err
		}
		decoded = append(decoded, decodedLayer{layer: layer, cfg: cfg, md: md})
	}

	merged := Config{
		// 与 Load 同一个理由：裸 bool 的默认值是 true，而零值是 false。
		// 这里先预置 true，下面只有哪一层真的写了 store_thought 才覆盖它。
		StoreThought: true,
		// language 的默认值同样不是零值：空串会被 applyDefaults 变成 auto，
		// 但先写出来，任何一层都没定义时合并结果与 Load 的默认一致。
		Language:  LanguageAuto,
		Providers: map[string]ProviderConfig{},
		Agents:    map[string]AgentConfig{},
		Teams:     map[string]TeamConfig{},
	}
	owner := provenance{
		agents:    map[string]string{},
		providers: map[string]string{},
		teams:     map[string]string{},
	}
	notices := make([]string, 0, 1)

	// 从低优先级到高优先级覆盖：同名 id 后写覆盖先写，写了的标量后写覆盖先写。
	// 这样「继承」就是「没人覆盖」，不需要在每个字段上再写一遍缺省判断。
	for i := len(decoded) - 1; i >= 0; i-- {
		d := decoded[i]
		path := d.layer.Path
		if d.md.IsDefined("data_dir") {
			if d.layer.Project {
				notices = append(notices, fmt.Sprintf(
					"项目配置 %s 里的 data_dir 被忽略：data_dir 只从 --config、环境变量 %s 或 ~/.council/config.toml 取",
					path, EnvConfigPath))
			} else {
				merged.DataDir = d.cfg.DataDir
				owner.dataDir = path
			}
		}
		if d.md.IsDefined("rules_file") {
			merged.RulesFile = d.cfg.RulesFile
			owner.rulesFileBase = filepath.Dir(path)
		}
		if d.md.IsDefined("store_thought") {
			merged.StoreThought = d.cfg.StoreThought
		}
		// language 与 store_thought 同一条规则：高优先级层写了才覆盖。
		// 项目层写 en、全局层没写时，合并结果就是 en。
		if d.md.IsDefined("language") {
			merged.Language = d.cfg.Language
			owner.language = path
		}
		if d.md.IsDefined("summary", "agent") {
			merged.Summary.Agent = d.cfg.Summary.Agent
			owner.summary = path
		}
		for id, provider := range d.cfg.Providers {
			merged.Providers[id] = provider
			owner.providers[id] = path
		}
		for id, agent := range d.cfg.Agents {
			merged.Agents[id] = agent
			owner.agents[id] = path
		}
		for id, team := range d.cfg.Teams {
			merged.Teams[id] = team
			owner.teams[id] = path
		}
	}

	applyDefaults(&merged)
	ensureMaps(&merged)
	// 校验必须跑在合并之后而不是每层各跑一次：项目层本来就可以只写
	// [agents.x] 而引用全局层的 provider，逐层校验会把这种合法写法判死。
	//
	// 只有真的合并了多层才补「来自 <文件>」。单层时那份文件就是唯一的一份，
	// 补一句话不会增加任何信息，却会让 LoadMerged 的错误文案与单文件
	// Load/Validate 分叉——repl 的 /add 写后校验与 reload 的校验错误都逐字
	// 依赖那份措辞（既有测试钉住），分叉就得放宽那些断言。
	if err := merged.Validate(); err != nil {
		if len(decoded) == 1 {
			return MergedConfig{}, err
		}
		return MergedConfig{}, decorateValidationErrors(err, owner)
	}

	result := MergedConfig{
		Config:        &merged,
		Layers:        make([]string, len(decoded)),
		RulesFileBase: owner.rulesFileBase,
		Notices:       notices,
	}
	for i, d := range decoded {
		result.Layers[i] = d.layer.Path
	}
	if result.RulesFileBase == "" {
		// 没有任何一层写 rules_file 时 cfg.RulesFile 是空串，LoadRules 会
		// 直接返回，这个值用不到；仍给它一个确定值，免得调用方拿到空串。
		result.RulesFileBase = filepath.Dir(layers[0].Path)
	}
	return result, nil
}

// provenance 记录每个值最终来自哪一份文件，只用于在校验错误里补一句出处。
type provenance struct {
	agents        map[string]string
	providers     map[string]string
	teams         map[string]string
	summary       string
	dataDir       string
	language      string
	rulesFileBase string
}

// sourceFor 找 path 指向的那个值来自哪份文件。找不到时返回空串（错误原样输出）。
//
// id 里可以含点号，所以不能按点号切段再回组：只能在已知 id 里找最长匹配，
// 且要求匹配在段边界上结束（a.b 不能把 a.bc 吃掉）。
func (p provenance) sourceFor(path string) string {
	tables := []struct {
		prefix string
		ids    map[string]string
	}{
		{"agents.", p.agents},
		{"providers.", p.providers},
		{"teams.", p.teams},
	}
	for _, table := range tables {
		rest, ok := strings.CutPrefix(path, table.prefix)
		if !ok {
			continue
		}
		bestID, bestPath := "", ""
		for id, src := range table.ids {
			if rest != id && !strings.HasPrefix(rest, id+".") {
				continue
			}
			if len(id) > len(bestID) {
				bestID, bestPath = id, src
			}
		}
		if bestPath != "" {
			return bestPath
		}
	}
	if path == "summary" || strings.HasPrefix(path, "summary.") {
		return p.summary
	}
	if path == "data_dir" {
		return p.dataDir
	}
	if path == "language" {
		return p.language
	}
	return ""
}

// decorateValidationErrors 给每条校验错误补上「来自 <文件>」。
//
// 只在合并了多层时调用（见 LoadMerged 里那一段）：单层没有歧义，补出处会
// 让错误文案与单文件 Load/Validate 的既有措辞分叉。
//
// 不新增错误类型、不改 Validate：ValidationErrors.Error() 的排序与格式
// 原样保留，只在 Msg 末尾追加一句。调用方（app / serve）照旧把它当普通
// error 打印，不需要认识任何新类型。
func decorateValidationErrors(err error, owner provenance) error {
	verrs, ok := err.(ValidationErrors)
	if !ok {
		return err
	}
	out := make(ValidationErrors, 0, len(verrs))
	for _, e := range verrs {
		if src := owner.sourceFor(e.Path); src != "" {
			e.Msg += "（来自 " + src + "）"
		}
		out = append(out, e)
	}
	return out
}
