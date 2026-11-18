package text

// SingleCountPorts 为计数入口提供一次选择，适配器管理提供商和上游资源并返回可用性及错误。
type SingleCountPorts interface {
	Select() (bool, error)
	Selected()
	SelectionFailed(error)
	Forward() error
	ForwardFailed(error)
}

// RunSingleCountTokens 不给原本无切号的 OpenAI 计数入口添加重试或费用路径。
func RunSingleCountTokens(p SingleCountPorts) {
	available, err := p.Select()
	p.Selected()
	if err != nil || !available {
		p.SelectionFailed(err)
		return
	}
	if err := p.Forward(); err != nil {
		p.ForwardFailed(err)
	}
}
