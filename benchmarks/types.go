package benchmarks

// DeepConfig has three levels of nesting so reflection's recursive walk
// is exercised beyond ParityConfig's single nested Server block.
type DeepConfig struct {
	L1 struct {
		Label string `cfg:"label" default:"l1"`
		L2    struct {
			Flag bool `cfg:"flag" default:"true"`
			L3   struct {
				Name string `cfg:"name" default:"leaf"`
				Port int    `cfg:"port" validate:"required,min=1,max=65535"`
				Host string `cfg:"host" default:"127.0.0.1"`
			} `cfg:"l3"`
		} `cfg:"l2"`
	} `cfg:"l1"`
	Top string `cfg:"top" default:"ok"`
}

// LargeSliceConfig isolates a single large []int bind path.
type LargeSliceConfig struct {
	Ports []int `cfg:"ports"`
}
