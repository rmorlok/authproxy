package meta

import "time"

func NewObjectReference(typeMeta TypeMeta, metadata ObjectMeta) ObjectReference {
	return ObjectReference{
		APIVersion: typeMeta.APIVersion,
		Kind:       typeMeta.Kind,
		ID:         metadata.ID,
		Name:       metadata.Name,
		Namespace:  metadata.Namespace,
		Generation: metadata.Generation,
	}
}

func NewCondition(conditionType string, status ConditionStatus, observedGeneration uint64, reason, message string, transitionTime time.Time) Condition {
	return Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: observedGeneration,
		LastTransitionTime: transitionTime.UTC(),
		Reason:             reason,
		Message:            message,
	}
}

// UpsertCondition replaces a condition with the same type or appends it. A
// status that did not transition retains its previous transition timestamp.
func UpsertCondition(conditions []Condition, next Condition) []Condition {
	result := append([]Condition(nil), conditions...)
	for i := range result {
		if result[i].Type != next.Type {
			continue
		}
		if result[i].Status == next.Status {
			next.LastTransitionTime = result[i].LastTransitionTime
		}
		result[i] = next
		return result
	}
	return append(result, next)
}
