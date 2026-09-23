import {Routes} from '@angular/router';
import {AuditPageComponent} from '../pages/audit.page';
import {KilnsPageComponent} from '../pages/kilns.page';
import {LotsPageComponent} from '../pages/lots.page';
import {ReadingsPageComponent} from '../pages/readings.page';
import {SchedulesPageComponent} from '../pages/schedules.page';

export const routes: Routes = [
  {path: 'kilns', component: KilnsPageComponent}, {path: 'lots', component: LotsPageComponent},
  {path: 'readings', component: ReadingsPageComponent}, {path: 'schedules', component: SchedulesPageComponent},
  {path: 'audit', component: AuditPageComponent}, {path: '', pathMatch: 'full', redirectTo: 'kilns'}, {path: '**', redirectTo: 'kilns'},
];
