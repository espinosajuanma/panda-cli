package catalog

type Kind string

const (
	Labels   Kind = "labels"
	Packages Kind = "packages"
)

func ItemName(kind Kind) string {
	if kind == Labels {
		return "label"
	}
	return "package"
}

func NameField(kind Kind) string {
	if kind == Labels {
		return "name"
	}
	return "title"
}

func Fields(kind Kind) []string {
	if kind == Labels {
		return []string{"name", "description"}
	}
	return []string{"title", "description", "startDate", "estimation"}
}
