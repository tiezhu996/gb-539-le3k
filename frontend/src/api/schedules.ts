import {request} from './client';
import type {DryingSchedule} from '../types/entities';

export const scheduleApi = {
  list: () => request<DryingSchedule[]>('/schedules'),
  get: (id: string) => request<DryingSchedule>(`/schedules/${id}`),
  calculate: (lotId: string) => request<DryingSchedule>('/schedules/calculate', {method: 'POST', body: JSON.stringify({timber_lot_id: lotId})}),
  review: (id: string, decision: string, note = '', version: number) => request<DryingSchedule>(`/schedules/${id}/review`, {method: 'POST', body: JSON.stringify({decision, note, version})}),
  freeze: (id: string, version: number) => request<DryingSchedule>(`/schedules/${id}/freeze`, {method: 'POST', body: JSON.stringify({version})}),
  compare: (id: string, baselineScheduleId: string) => request<unknown>(`/schedules/${id}/compare`, {method: 'POST', body: JSON.stringify({baseline_schedule_id: baselineScheduleId})}),
};
