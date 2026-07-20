import axios from 'axios'
import { expect, it } from 'vitest'

it('serializes special label values through Axios params encoding', () => {
  const uri = axios.getUri({url:'/observability/datasources/3/label-values',params:{label:'pod/name + zone'}})
  expect(uri).toBe('/observability/datasources/3/label-values?label=pod%2Fname+%2B+zone')
})
