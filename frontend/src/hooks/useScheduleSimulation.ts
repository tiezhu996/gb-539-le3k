import {scheduleApi} from '../api/schedules'; export function useScheduleSimulation(){return {calculate:(lotId:string)=>scheduleApi.calculate(lotId)}}
