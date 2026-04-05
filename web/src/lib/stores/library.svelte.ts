let currentSort = $state<string>('added_at');
let currentOrder = $state<string>('desc');

export function getSort(): string {
  return currentSort;
}

export function getOrder(): string {
  return currentOrder;
}

export function setSort(sort: string) {
  currentSort = sort;
}

export function setOrder(order: string) {
  currentOrder = order;
}
