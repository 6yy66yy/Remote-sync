package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Group 服务器下的路径组合
type Group struct {
	Name       string `yaml:"name"`
	RemotePath string `yaml:"remote_path"`
	LocalPath  string `yaml:"local_path"`
}

// Server 服务器配置
type Server struct {
	Name        string   `yaml:"name"`
	Host        string   `yaml:"host"`
	Port        int      `yaml:"port"`
	Username    string   `yaml:"username"`
	Password    string   `yaml:"password"`
	KeyPath     string   `yaml:"key_path"`
	Groups      []Group  `yaml:"groups"`
	Concurrency int      `yaml:"concurrency"` // 并发上传限制
}

// Config 全局配置
type Config struct {
	Servers []Server `yaml:"servers"`
}

// DefaultConfigPath 返回默认配置文件路径
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".remotesync", "config.yaml")
}

// Load 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Servers: []Server{}}, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}

// Save 保存配置到 YAML 文件
func Save(path string, cfg *Config) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// FindServer 根据名称查找服务器
func (c *Config) FindServer(name string) *Server {
	for i := range c.Servers {
		if c.Servers[i].Name == name {
			return &c.Servers[i]
		}
	}
	return nil
}

// AddServer 添加服务器
func (c *Config) AddServer(server Server) {
	c.Servers = append(c.Servers, server)
}

// RemoveServer 删除服务器
func (c *Config) RemoveServer(name string) bool {
	for i, s := range c.Servers {
		if s.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			return true
		}
	}
	return false
}

// FindGroup 在服务器中根据名称查找组合
func (s *Server) FindGroup(name string) *Group {
	for i := range s.Groups {
		if s.Groups[i].Name == name {
			return &s.Groups[i]
		}
	}
	return nil
}

// AddGroup 添加组合
func (s *Server) AddGroup(group Group) {
	s.Groups = append(s.Groups, group)
}

// RemoveGroup 删除组合
func (s *Server) RemoveGroup(name string) bool {
	for i, g := range s.Groups {
		if g.Name == name {
			s.Groups = append(s.Groups[:i], s.Groups[i+1:]...)
			return true
		}
	}
	return false
}
