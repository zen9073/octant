package printer

import (
	"context"
	"fmt"

	"github.com/heptio/developer-dash/internal/overview/link"

	"github.com/heptio/developer-dash/internal/cache"
	"github.com/heptio/developer-dash/internal/view/component"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
)

// StatefulSetListHandler is a printFunc that list stateful sets
func StatefulSetListHandler(ctx context.Context, list *appsv1.StatefulSetList, opts Options) (component.ViewComponent, error) {
	if list == nil {
		return nil, errors.New("nil list")
	}

	cols := component.NewTableCols("Name", "Labels", "Desired", "Current", "Age", "Selector")
	tbl := component.NewTable("StatefulSets", cols)

	for _, statefulSet := range list.Items {
		row := component.TableRow{}
		row["Name"] = link.ForObject(&statefulSet, statefulSet.Name)
		row["Labels"] = component.NewLabels(statefulSet.Labels)

		desired := fmt.Sprintf("%d", *statefulSet.Spec.Replicas)
		row["Desired"] = component.NewText(desired)

		current := fmt.Sprintf("%d", statefulSet.Status.Replicas)
		row["Current"] = component.NewText(current)

		ts := statefulSet.CreationTimestamp.Time
		row["Age"] = component.NewTimestamp(ts)

		row["Selector"] = printSelector(statefulSet.Spec.Selector)

		tbl.Add(row)
	}

	return tbl, nil
}

// StatefulSetHandler is a printFunc that prints a StatefulSet
func StatefulSetHandler(ctx context.Context, statefulSet *appsv1.StatefulSet, options Options) (component.ViewComponent, error) {
	o := NewObject(statefulSet)

	o.RegisterConfig(func() (component.ViewComponent, error) {
		statefulSetConfigGen := NewStatefulSetConfiguration(statefulSet)
		return statefulSetConfigGen.Create()
	}, 16)

	o.RegisterSummary(func() (component.ViewComponent, error) {
		statefulSetSummaryGen := NewStatefulSetStatus(statefulSet)
		return statefulSetSummaryGen.Create(ctx, options.Cache)
	}, 8)

	o.EnablePodTemplate(statefulSet.Spec.Template)

	o.RegisterItems(ItemDescriptor{
		Func: func() (component.ViewComponent, error) {
			return createPodListView(ctx, statefulSet, options)
		},
		Width: component.WidthFull,
	})

	o.EnableEvents()

	return o.ToComponent(ctx, options)
}

// StatefulSetConfiguration generates a statefulset configuration
type StatefulSetConfiguration struct {
	statefulset *appsv1.StatefulSet
}

// NewStatefulSetConfiguration creates an instance of StatefulSetconfiguration
func NewStatefulSetConfiguration(statefulSet *appsv1.StatefulSet) *StatefulSetConfiguration {
	return &StatefulSetConfiguration{
		statefulset: statefulSet,
	}
}

// Create generates a statefulset configuration summary
func (sc *StatefulSetConfiguration) Create() (*component.Summary, error) {
	if sc == nil || sc.statefulset == nil {
		return nil, errors.New("statefulset is nil")
	}

	statefulSet := sc.statefulset

	sections := component.SummarySections{}

	sections.AddText("Update Strategy", string(statefulSet.Spec.UpdateStrategy.Type))

	if selector := statefulSet.Spec.Selector; selector != nil {
		var selectors []component.Selector

		for _, lsr := range selector.MatchExpressions {
			o, err := component.MatchOperator(string(lsr.Operator))
			if err != nil {
				return nil, err
			}

			es := component.NewExpressionSelector(lsr.Key, o, lsr.Values)
			selectors = append(selectors, es)
		}

		for k, v := range selector.MatchLabels {
			ls := component.NewLabelSelector(k, v)
			selectors = append(selectors, ls)
		}

		sections = append(sections, component.SummarySection{
			Header:  "Selectors",
			Content: component.NewSelectors(selectors),
		})
	}

	total := fmt.Sprintf("%d", statefulSet.Status.Replicas)

	if desired := statefulSet.Spec.Replicas; desired != nil {
		desired := fmt.Sprintf("%d", *desired)
		status := fmt.Sprintf("%s Desired / %s Total", desired, total)
		sections.AddText("Replicas", status)
	}

	sections.AddText("Pod Management Policy", string(statefulSet.Spec.PodManagementPolicy))

	summary := component.NewSummary("Configuration", sections...)
	return summary, nil
}

// StatefulSetStatus generates a statefulset status
type StatefulSetStatus struct {
	statefulset *appsv1.StatefulSet
}

// NewStatefulSetStatus creates an instance of StatefulSetStatus
func NewStatefulSetStatus(statefulSet *appsv1.StatefulSet) *StatefulSetStatus {
	return &StatefulSetStatus{
		statefulset: statefulSet,
	}
}

// Create generates a statefulset status quadrant
func (statefulSet *StatefulSetStatus) Create(ctx context.Context, c cache.Cache) (*component.Quadrant, error) {
	if statefulSet.statefulset == nil {
		return nil, errors.New("statefulset is nil")
	}

	pods, err := listPods(ctx, statefulSet.statefulset.ObjectMeta.Namespace, statefulSet.statefulset.Spec.Selector, statefulSet.statefulset.GetUID(), c)
	if err != nil {
		return nil, errors.Wrap(err, "list pods")
	}

	ps := createPodStatus(pods)

	quadrant := component.NewQuadrant()
	if err := quadrant.Set(component.QuadNW, "Running", fmt.Sprintf("%d", ps.Running)); err != nil {
		return nil, errors.New("unable to set quadrant nw")
	}
	if err := quadrant.Set(component.QuadNE, "Waiting", fmt.Sprintf("%d", ps.Waiting)); err != nil {
		return nil, errors.New("unable to set quadrant ne")
	}
	if err := quadrant.Set(component.QuadSW, "Succeeded", fmt.Sprintf("%d", ps.Succeeded)); err != nil {
		return nil, errors.New("unable to set quadrant sw")
	}
	if err := quadrant.Set(component.QuadSE, "Failed", fmt.Sprintf("%d", ps.Failed)); err != nil {
		return nil, errors.New("unable to set quadrant se")
	}

	return quadrant, nil
}
