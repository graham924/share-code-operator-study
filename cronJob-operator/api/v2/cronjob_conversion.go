package v2

import (
	"fmt"
	v1 "graham924.com/cronJob-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"strings"
)

func (src *CronJob) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1.CronJob)

	sched := src.Spec.Schedule
	scheduleParts := []string{"*", "*", "*", "*", "*"}
	if sched.Minute != nil {
		scheduleParts[0] = string(*sched.Minute)
	}
	if sched.Hour != nil {
		scheduleParts[1] = string(*sched.Hour)
	}
	if sched.DayOfMonth != nil {
		scheduleParts[2] = string(*sched.DayOfMonth)
	}
	if sched.Month != nil {
		scheduleParts[3] = string(*sched.Month)
	}
	if sched.DayOfWeek != nil {
		scheduleParts[4] = string(*sched.DayOfWeek)
	}
	dst.Spec.Schedule = strings.Join(scheduleParts, " ")
	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.StartingDeadlineSeconds = src.Spec.StartingDeadlineSeconds
	dst.Spec.ConcurrencyPolicy = v1.ConcurrencyPolicy(src.Spec.ConcurrencyPolicy)
	dst.Spec.Suspend = src.Spec.Suspend
	dst.Spec.JobTemplate = src.Spec.JobTemplate
	dst.Spec.SuccessfulJobsHistoryLimit = src.Spec.SuccessfulJobsHistoryLimit
	dst.Spec.FailedJobsHistoryLimit = src.Spec.FailedJobsHistoryLimit

	// Status
	dst.Status.Active = src.Status.Active
	dst.Status.LastScheduleTime = src.Status.LastScheduleTime

	return nil
}
func (dst *CronJob) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1.CronJob)
	schedParts := strings.Split(src.Spec.Schedule, " ")
	if len(schedParts) != 5 {
		return fmt.Errorf("invalid schedule: not a standard 5-field schedule")
	}

	partIfNeeded := func(raw string) *CronField {
		if raw == "*" {
			return nil
		}
		part := CronField(raw)
		return &part
	}

	dst.Spec.Schedule = CronSchedule{
		Minute:     partIfNeeded(schedParts[0]),
		Hour:       partIfNeeded(schedParts[1]),
		DayOfMonth: partIfNeeded(schedParts[2]),
		Month:      partIfNeeded(schedParts[3]),
		DayOfWeek:  partIfNeeded(schedParts[4]),
	}

	/*
		The rest of the conversion is pretty rote.
	*/
	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec
	dst.Spec.StartingDeadlineSeconds = src.Spec.StartingDeadlineSeconds
	dst.Spec.ConcurrencyPolicy = ConcurrencyPolicy(src.Spec.ConcurrencyPolicy)
	dst.Spec.Suspend = src.Spec.Suspend
	dst.Spec.JobTemplate = src.Spec.JobTemplate
	dst.Spec.SuccessfulJobsHistoryLimit = src.Spec.SuccessfulJobsHistoryLimit
	dst.Spec.FailedJobsHistoryLimit = src.Spec.FailedJobsHistoryLimit

	// Status
	dst.Status.Active = src.Status.Active
	dst.Status.LastScheduleTime = src.Status.LastScheduleTime

	return nil
}
