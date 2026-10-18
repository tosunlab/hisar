import { useCachedState } from "@raycast/utils";
import { useHisar } from "./raycast/hooks";
import { allItemsFilterValue } from "./raycast/itemList";
import { ItemListView } from "./raycast/itemListView";

export default function SearchHisar() {
  const hisar = useHisar();
  // Only the dropdown's selection is cached on disk; the items stay in memory.
  const [filterValue, setFilterValue] = useCachedState("itemFilter", allItemsFilterValue);
  return <ItemListView hisar={hisar} filterValue={filterValue} onFilterChange={setFilterValue} />;
}
