const SORT_KEY = 'swipefi_sort';
const ORDER_KEY = 'swipefi_order';

let currentSort = $state<string>(localStorage.getItem(SORT_KEY) || 'added_at');
let currentOrder = $state<string>(localStorage.getItem(ORDER_KEY) || 'desc');

export function getSort(): string {
  return currentSort;
}

export function getOrder(): string {
  return currentOrder;
}

export function setSort(sort: string) {
  currentSort = sort;
  localStorage.setItem(SORT_KEY, sort);
}

export function setOrder(order: string) {
  currentOrder = order;
  localStorage.setItem(ORDER_KEY, order);
}
