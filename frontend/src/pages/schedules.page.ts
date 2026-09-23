import {Component, inject} from '@angular/core';
import {NgFor} from '@angular/common';
import {AppStore} from '../stores/app.store';
import {scheduleApi} from '../api/schedules';
import {RuleEvidenceDrawerComponent} from '../components/common/RuleEvidenceDrawer';
import {expiryReasonLabel} from '../utils/format';
import type {DryingSchedule} from '../types/entities';

@Component({
  standalone: true,
  imports: [NgFor, RuleEvidenceDrawerComponent],
  template: `<div class="page-head"><div><p class="eyebrow">04 / SCHEDULES</p><h1>曲线优化</h1><p class="description">把算法建议放在规则证据和窑炉边界旁，支持人工复核；读数变化后旧计划自动过期并与新计划互链。</p></div><button class="button primary" (click)="calculate()">运行一次仿真</button></div>
<div class="schedule-banner"><div class="banner-mark">∿</div><div><strong>算法版本 curve-v2.0</strong><p>同一输入哈希会复用最新结果；读数补录、作废或修正会把同批次旧计划标为过期，已冻结计划仅保留快照，过期计划不能再审核或冻结。</p></div></div>
<p class="form-error">{{error}}</p>
<p class="schedule-tip" *ngIf="expiredCount() > 0"><strong>{{expiredCount()}}</strong> 个计划已随读数变化过期；批次进入完成前，最新计划必须为<strong>已冻结</strong>状态。</p>
<section class="workspace-panel"><div class="panel-head"><div><h2>计划版本</h2><p>冻结计划可作为后续比较的不可变基线；过期计划保留审计快照和替代链。</p></div><span class="count">{{store.schedules().length}} 个版本</span></div><div class="table-wrap"><table>
<thead><tr><th>计划</th><th>批次</th><th>风险</th><th>生命周期</th><th>过期与替代</th></tr></thead>
<tbody><tr *ngFor="let item of store.schedules()">
  <td><strong>{{item.id.slice(0,8)}}</strong><small>{{item.algorithm_version}} · v{{item.version}}</small></td>
  <td>{{item.timber_lot_id.slice(0,8)}}</td>
  <td><span class="risk low">{{item.defect_risk_score}} / 100</span></td>
  <td>
    <span [class.state]="true" [class.warm]="!isExpired(item)" [class.expired]="isExpired(item)">{{item.schedule_state}}</span>
    <small class="frozen-note" *ngIf="item.frozen_at && !isExpired(item)">已冻结快照 · {{(item.frozen_by||'').slice(0,8)}}</small>
    <small class="frozen-note" *ngIf="item.frozen_at && isExpired(item)">冻结快照已归档保留</small>
  </td>
  <td>
    <ng-container *ngIf="isExpired(item)">
      <span class="state expired">已过期</span>
      <small class="expiry-note">{{expiryLabel(item)}}</small>
      <small class="replace-note" *ngIf="item.superseded_by_id">已被替代 → {{item.superseded_by_id.slice(0,8)}}</small>
      <small class="replace-note" *ngIf="!item.superseded_by_id">等待重新计算的计划接管</small>
    </ng-container>
    <small class="replace-note" *ngIf="!isExpired(item) && item.supersedes_id">替代旧计划 ← {{item.supersedes_id.slice(0,8)}}</small>
    <small class="replace-note" *ngIf="!isExpired(item) && !item.supersedes_id">当前最新计划</small>
  </td>
</tr></tbody></table></div></section>`,
})
export class SchedulesPageComponent {
  readonly store = inject(AppStore);
  error = '';
  expiryLabel = (item: DryingSchedule) => expiryReasonLabel(item.expiry_reason ?? '');
  isExpired = (item: DryingSchedule) => !!item.expired_at;
  expiredCount = () => this.store.schedules().filter((item) => !!item.expired_at).length;
  async calculate() {
    const lot = this.store.lots()[0];
    if (!lot) { this.error = '请先登记批次'; return; }
    try {
      await scheduleApi.calculate(lot.id);
      await this.store.refresh();
      this.error = '';
    } catch (error) {
      this.error = error instanceof Error ? error.message : '计算失败';
    }
  }
}
