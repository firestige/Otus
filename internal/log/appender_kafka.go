package log

type KafkaAppenderOpt struct {
	Brokers   []string `yaml:"brokers"`
	Topic     string   `yaml:"topic"`
	Partition int      `yaml:"partition,omitempty"`
	// 可选的认证配置
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
	TLS      bool   `yaml:"tls,omitempty"`
}

func (m *MultiWriter) AddKafkaAppender(options KafkaAppenderOpt) *MultiWriter {
	// 实现Kafka appender逻辑
	// 创建Kafka producer并添加到MultiWriter
	return m
}
