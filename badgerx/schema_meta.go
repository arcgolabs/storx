package badgerx

// IndexKind identifies one schema index shape.
type IndexKind string

const (
	IndexKindUnique  IndexKind = "unique"
	IndexKindMany    IndexKind = "many"
	IndexKindOrdered IndexKind = "ordered"
)

// RelationKind identifies one model relation shape.
type RelationKind string

const (
	RelationKindBelongsTo RelationKind = "belongs_to"
	RelationKindHasOne    RelationKind = "has_one"
	RelationKindHasMany   RelationKind = "has_many"
)

// RelationDefinition declares relation metadata for one model schema.
type RelationDefinition struct {
	Name         string
	Kind         RelationKind
	TargetModel  string
	LocalIndex   string
	ForeignIndex string
}

// SchemaIndexDescription describes one declared index.
type SchemaIndexDescription struct {
	Name          string
	Kind          IndexKind
	SecondaryType string
	SortType      string
	Unique        bool
}

// SchemaRelationDescription describes one declared relation.
type SchemaRelationDescription struct {
	Name         string
	Kind         RelationKind
	TargetModel  string
	LocalIndex   string
	ForeignIndex string
}

// SchemaDescription describes one declared model schema.
type SchemaDescription struct {
	Prefix         string
	PrimaryKeyType string
	ValueType      string
	Indexes        []SchemaIndexDescription
	Relations      []SchemaRelationDescription
}

type schemaMetadataProvider interface {
	describeIndex() SchemaIndexDescription
}

// Describe returns declarative metadata for one model schema.
func (s ModelSchema[K, V]) Describe() SchemaDescription {
	description := SchemaDescription{
		Prefix:         s.Prefix,
		PrimaryKeyType: typeOf[K](),
		ValueType:      typeOf[V](),
		Indexes:        make([]SchemaIndexDescription, 0, len(s.Indexes)),
		Relations:      make([]SchemaRelationDescription, 0, len(s.Relations)),
	}

	for _, index := range s.Indexes {
		describer, ok := index.(schemaMetadataProvider)
		if !ok {
			continue
		}
		description.Indexes = append(description.Indexes, describer.describeIndex())
	}
	for _, relation := range s.Relations {
		description.Relations = append(description.Relations, SchemaRelationDescription{
			Name:         relation.Name,
			Kind:         relation.Kind,
			TargetModel:  relation.TargetModel,
			LocalIndex:   relation.LocalIndex,
			ForeignIndex: relation.ForeignIndex,
		})
	}
	return description
}

func (d SecondaryIndexDefinition[K, V, IK]) describeIndex() SchemaIndexDescription {
	return SchemaIndexDescription{
		Name:          d.Prefix,
		Kind:          IndexKindUnique,
		SecondaryType: typeOf[IK](),
		Unique:        true,
	}
}

func (d SecondaryIndexManyDefinition[K, V, IK]) describeIndex() SchemaIndexDescription {
	return SchemaIndexDescription{
		Name:          d.Prefix,
		Kind:          IndexKindMany,
		SecondaryType: typeOf[IK](),
	}
}
