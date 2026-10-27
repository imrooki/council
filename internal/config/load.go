package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load 读文件、解析 TOML、拒绝未知键、填缺省值、把 Permissions、Mode 与 thought_visible_to 规范成小写。
// 不做跨引用校验。
func Load(path string) (*Config, error) {
	// StoreThought 默认 true，但它是裸 bool：toml.Decode 只在用户写了这个键时
	// 才覆盖字段，没写就保留这里预置的值。若从 cfg := Config{} 的自然零值
	// false 开始 Decode，用户省略该键时读出的就是 false —— 正好是默认值的
	// 反面，而且没有任何报错。所以必须在 Decode 之前把默认值填好；
	// 用户若显式写了 store_thought = false，Decode 会原样覆盖这里的预置值。
	cfg := Config{StoreThought: true}
	if _, err := decodeConfigFile(path, &cfg); err != nil {
		return nil, err
	}

	applyDefaults(&cfg)
	ensureMaps(&cfg)
	return &cfg, nil
}

// decodeConfigFile 读、解析一份 TOML 并拒绝未知键，把结果填进 cfg。
//
// 不做缺省填充、不做校验：Load 与 LoadMerged 对「缺省值什么时候生效」
// 有不同要求 —— Load 必须让 StoreThought 的预置值参与 Decode，合并则要
// 先按层判断某个键写没写、再决定谁覆盖谁。把这三步拆开，两边共用同一份
// 读文件 / 报错 / 查未知键的实现，错误文案也只有一个出处。
func decodeConfigFile(path string, cfg *Config) (toml.MetaData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return toml.MetaData{}, fmt.Errorf("read config file %s: %w", path, err)
	}
	md, err := toml.Decode(string(data), cfg)
	if err != nil {
		return toml.MetaData{}, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if err := unknownKeysError(path, md); err != nil {
		return toml.MetaData{}, err
	}
	return md, nil
}

// LoadAndValidate 先 Load 再 Validate，任一步失败即返回该错误。
func LoadAndValidate(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func unknownKeysError(path string, md toml.MetaData) error {
	undecoded := md.Undecoded()
	if len(undecoded) == 0 {
		return nil
	}
	keys := make([]string, 0, len(undecoded))
	for _, key := range undecoded {
		keys = append(keys, strings.Join(key, "."))
	}
	sort.Strings(keys)
	msg := fmt.Sprintf("unknown config keys in %s: %s", path, strings.Join(keys, ", "))
	for _, hint := range dottedIDHints(undecoded) {
		msg += "\n" + hint
	}
	return errors.New(msg)
}

// dottedIDHints 为「id 含点号、表名却没加引号」给出引号写法的提示。
//
// 不加引号的 [agents.grok-4.7] 被 TOML 读成 agents -> grok-4 -> 7 三层，上面那句
// 报错把 agents.grok-4.7.model 原样回显，读不出问题在点号。这里只追加提示，
// 原报错一个字不改。
//
// 判据是三条同时成立：键长 >= 4、首段是 agents / teams / providers 之一、末段是
// 该表值类型的一个已知字段名。
//   - 长度 >= 4 是兜底，不是挡拼错字段的那一道：[agents.foo] 下拼错一个字段
//     （bogus = 1）得到长 3 的 [agents foo bogus]，那是真正的未知键；真正挡住它的是
//     下面的末段筛选（bogus 不是字段名）。长 3、末段又是已知字段名的 undecoded 键
//     不可能出现 —— 它要么被正常 decode，要么在 decode 阶段就以类型错误失败 ——
//     所以在末段筛选之下 >= 3 与 >= 4 行为相同（2026-09-23 变异实测）。留着 >= 4，
//     是为了哪天有人拿掉末段筛选时，拼错的字段仍不会被说成点号问题。
//   - 还要看末段：id 含两个以上点号时，表名键本身也长 >= 4（[agents.a.b.c] 得到
//     [agents a b c]），不筛末段就会推出一个错的 id a.b，还排在对的 a.b.c 前面；
//     表下写点号键（bogus.x = 1）也会产生长 4 的键。筛末段能去掉其中大部分，
//     但末段恰好是字段名的点号键或子表（bogus.provider = 1、[agents.foo.extra]
//     下写 provider）仍会命中。
//
// 所以提示必须是「如果……」的条件句，不能下断言。
//
// 已知限制，两条，都接受：表下一个字段都没有的 [agents.grok-4.7] 只产生长 3 的
// 键，不出提示；不加引号的点号 id 末段恰好与标量字段同名时（[agents.gpt.model]），
// TOML 在 decode 阶段就以类型错误失败，走不到这里。
//
// 同一个 (表, id) 只提示一次：一个表下通常有好几个字段，每个字段都会产生一条
// 同样前缀的未知键。按 id 字典序输出，同 id 再按表名，输出才稳定。
func dottedIDHints(undecoded []toml.Key) []string {
	fields := tableFieldNames()
	type hint struct{ table, id string }
	seen := make(map[hint]struct{})
	var hints []hint
	for _, key := range undecoded {
		if len(key) < 4 {
			continue
		}
		known, ok := fields[key[0]]
		if !ok {
			continue
		}
		if _, ok := known[key[len(key)-1]]; !ok {
			continue
		}
		h := hint{table: key[0], id: strings.Join(key[1:len(key)-1], ".")}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		hints = append(hints, h)
	}
	sort.Slice(hints, func(i, j int) bool {
		if hints[i].id != hints[j].id {
			return hints[i].id < hints[j].id
		}
		return hints[i].table < hints[j].table
	})
	out := make([]string, len(hints))
	for i, h := range hints {
		out[i] = fmt.Sprintf("如果 %s 本来就是一个含点号的 id：TOML 把不加引号的点号当成层级，表名要写成 [%s.\"%s\"]", h.id, h.table, h.id)
	}
	return out
}

// tableFieldNames 返回「表名 -> 该表值类型的已知字段名集合」，
// 表名与字段名都从 Config 及其值类型的 toml tag 用 reflect 取。
//
// 不手写清单：有人给 AgentConfig 加字段时，手写清单会悄悄漏掉提示，
// 而且没有任何东西会因此变红。
func tableFieldNames() map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{})
	root := reflect.TypeOf(Config{})
	for i := 0; i < root.NumField(); i++ {
		f := root.Field(i)
		// 只有「id -> 结构体」的 map 才是 [表.id] 这种写法，providers / agents /
		// teams 三张表正好是全部这样的字段。
		if f.Type.Kind() != reflect.Map || f.Type.Key().Kind() != reflect.String || f.Type.Elem().Kind() != reflect.Struct {
			continue
		}
		table := tomlTagName(f)
		if table == "" {
			continue
		}
		names := make(map[string]struct{})
		elem := f.Type.Elem()
		for j := 0; j < elem.NumField(); j++ {
			if name := tomlTagName(elem.Field(j)); name != "" {
				names[name] = struct{}{}
			}
		}
		out[table] = names
	}
	return out
}

// tomlTagName 取字段 toml tag 里的名字部分。tag 可能带 ,omitempty 这类选项，
// 所以按逗号截断；没写 tag 或写成 "-" 的字段不算已知字段。
func tomlTagName(f reflect.StructField) string {
	tag := f.Tag.Get("toml")
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	if tag == "-" {
		return ""
	}
	return tag
}

func applyDefaults(cfg *Config) {
	// 语言是唯一一个「空串 = 跟随系统」的顶层字段。这里只做归一化，
	// 合法性与取值（zh / en / auto）留给 Validate；非法值不能被默认值
	// 悄悄吃掉，否则用户写错一个字母会得到一种界面语言却没有任何提示。
	cfg.Language = strings.ToLower(strings.TrimSpace(cfg.Language))
	if cfg.Language == "" {
		cfg.Language = LanguageAuto
	}
	// local_agent 与 language 同一条归一化纪律：只折大小写与空白，
	// 合法性留给 Validate（写错一个字母不该被默认值悄悄吃掉）。
	// 但空串要落成 off 而不是留空：这个键的缺省语义就是「关」，
	// 让 Load 交出的配置里没有第三种表示，读取方（council who / MCP）
	// 只需要比较 LevelOf(cfg.LocalAgent)。
	cfg.LocalAgent = strings.ToLower(strings.TrimSpace(cfg.LocalAgent))
	if cfg.LocalAgent == "" {
		cfg.LocalAgent = LocalAgentOff
	}
	for id, agent := range cfg.Agents {
		if agent.Permissions == "" {
			agent.Permissions = PermissionRead
		} else {
			agent.Permissions = Permission(strings.ToLower(string(agent.Permissions)))
		}
		if agent.Mode == "" {
			agent.Mode = ModeDiscussion
		} else {
			agent.Mode = Mode(strings.ToLower(string(agent.Mode)))
		}
		cfg.Agents[id] = agent
	}
	// 只给总结成员这一个 agent 补默认 role，而且只补空值。
	// 放在 Load 而不是 Validate 或 turn：askUpToWith、budgetOne、AuditDrift、
	// /rules 的探针四处算指纹，读的都是 Load 交出的这一份配置，放这里四处自动一致。
	// agent 指向不存在的 id 时什么都不做，由 Validate 报。
	// 代价是不经 Load 构造的配置（测试）拿不到默认值。
	if id := cfg.Summary.Agent; id != "" {
		if agent, ok := cfg.Agents[id]; ok && agent.Role == "" {
			agent.Role = DefaultSummaryRole
			cfg.Agents[id] = agent
		}
	}
	for id, team := range cfg.Teams {
		for i, m := range team.ThoughtVisibleTo {
			team.ThoughtVisibleTo[i] = Mode(strings.ToLower(string(m)))
		}
		cfg.Teams[id] = team
	}
}

func ensureMaps(cfg *Config) {
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{}
	}
	if cfg.Teams == nil {
		cfg.Teams = map[string]TeamConfig{}
	}
}
