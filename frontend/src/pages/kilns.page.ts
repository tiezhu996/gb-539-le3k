import {Component, inject, OnInit} from '@angular/core';
import {NgFor, NgIf} from '@angular/common';
import {FormsModule} from '@angular/forms';
import {kilnApi} from '../api/kilns';
import {AppStore} from '../stores/app.store';

@Component({standalone:true, imports:[FormsModule, NgFor, NgIf], template:`
<div class="page-head"><div><p class="eyebrow">01 / KILNS</p><h1>窑炉档案</h1><p class="description">维护容量与安全边界，查看当前工艺负荷。</p></div><button class="button primary" (click)="showForm=!showForm">{{showForm ? '取消' : '＋ 新建窑炉'}}</button></div>
<form *ngIf="showForm" class="workspace-panel modal-form" (ngSubmit)="create()"><h2>新建窑炉</h2><div class="form-grid"><label>窑炉编号<input name="kilnCode" [(ngModel)]="draft.kiln_code" required></label><label>名称<input name="name" [(ngModel)]="draft.name" required></label></div><div class="form-grid"><label>容量 m³<input name="capacity" type="number" [(ngModel)]="draft.capacity_m3" required></label><label>最高温度 °C<input name="temperature" type="number" [(ngModel)]="draft.max_temperature_c" required></label></div><label>最低湿度 %<input name="humidity" type="number" [(ngModel)]="draft.min_humidity_pct" required></label><button class="button primary" type="submit">保存窑炉</button><p class="form-error">{{error}}</p></form>
<section class="workspace-panel"><div class="panel-head"><div><h2>窑炉清单</h2><p>每条边界均会冻结到曲线计划快照。</p></div><span class="count">{{store.kilns().length}} 台</span></div><div class="table-wrap"><table><thead><tr><th>窑炉</th><th>容量</th><th>温湿边界</th><th>状态</th></tr></thead><tbody><tr *ngFor="let kiln of store.kilns()"><td><strong>{{kiln.kiln_code}}</strong><small>{{kiln.name}}</small></td><td>{{kiln.capacity_m3}} m³</td><td>{{kiln.max_temperature_c}}°C / {{kiln.min_humidity_pct}}%</td><td><span class="state good">{{kiln.kiln_state}}</span></td></tr></tbody></table></div></section>`})
export class KilnsPageComponent implements OnInit {
  readonly store=inject(AppStore);
  showForm=false; error=''; draft={kiln_code:'K-02',name:'二号窑',capacity_m3:80,max_temperature_c:70,min_humidity_pct:35};
  ngOnInit(){void this.store.refresh()}
  async create(){try{await kilnApi.create({...this.draft, airflow_class:'uniform',owner_team:'干燥工艺组'});this.showForm=false;await this.store.refresh()}catch(error){this.error=error instanceof Error?error.message:'保存失败'}}
}
