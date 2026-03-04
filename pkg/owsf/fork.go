package owsf

func ForkDocument(parent Document, atEventID string, kind string) Document {
	child := NewDocument(parent.Producer.Name, parent.Producer.Version)
	child.Fork = &ForkRef{
		ParentDocID:   parent.DocID,
		ParentEventID: atEventID,
		Kind:          kind,
	}
	return child
}

func ValidateNoMerge(parentIDs []string) error {
	if len(parentIDs) > 1 {
		return ErrMultiParent
	}
	return nil
}

func CombineAsNewDocument(name, version string, inputDocIDs []string) Document {
	doc := NewDocument(name, version)
	doc.InputRefs = inputDocIDs
	return doc
}
