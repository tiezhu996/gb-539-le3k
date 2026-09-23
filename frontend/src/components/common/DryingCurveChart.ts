import {Component, Input} from '@angular/core';
@Component({selector: 'kc-drying-curve', standalone: true, template: '<svg class="curve" viewBox="0 0 240 110" role="img" aria-label="含水率趋势"><polyline [attr.points]="points"></polyline></svg>'})
export class DryingCurveChartComponent {
  @Input() values: number[] = [];
  get points(): string { return this.values.length ? this.values.map((value, index) => `${index * 60},${Math.max(8, 100 - value * 2)}`).join(' ') : '0,92 80,70 160,48 240,30'; }
}
