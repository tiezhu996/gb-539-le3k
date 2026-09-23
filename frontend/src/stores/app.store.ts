import {inject, Injectable, NgZone, signal} from '@angular/core';
import {auditApi, type AuditEvent} from '../api/audit';
import {kilnApi} from '../api/kilns';
import {lotApi} from '../api/lots';
import {readingApi} from '../api/readings';
import {scheduleApi} from '../api/schedules';
import type {DryingKiln, TimberLot, MoistureReading, DryingSchedule} from '../types/entities';

@Injectable({providedIn: 'root'})
export class AppStore {
  private readonly zone = inject(NgZone);
  readonly kilns = signal<DryingKiln[]>([]);
  readonly lots = signal<TimberLot[]>([]);
  readonly readings = signal<MoistureReading[]>([]);
  readonly schedules = signal<DryingSchedule[]>([]);
  readonly audits = signal<AuditEvent[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');

  async refresh(): Promise<void> {
    this.loading.set(true); this.error.set('');
    try {
      const [kilns, lots, readings, schedules, audits] = await Promise.all([kilnApi.list(), lotApi.list(), readingApi.list(), scheduleApi.list(), auditApi.list()]);
      this.zone.run(() => { this.kilns.set(kilns); this.lots.set(lots); this.readings.set(readings); this.schedules.set(schedules); this.audits.set(audits); });
    } catch (error) { this.zone.run(() => this.error.set(error instanceof Error ? error.message : '暂时无法读取工艺数据')); }
    finally { this.zone.run(() => this.loading.set(false)); }
  }
}
