package config

type Flag struct {
	File   string
	Config *Configuration
	IsSet  bool
}

func (f *Flag) Set(path string) error {
	f.File = path

	cfg, err := FromFile(path)
	if err != nil {
		return err
	}

	*f.Config = cfg
	f.IsSet = true

	return nil
}

func (f *Flag) String() string {
	return f.File
}
