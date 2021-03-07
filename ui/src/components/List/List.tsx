import React, {PropsWithChildren, ReactNode} from "react";

export interface ListProps<T> {
	dataSource: T[];
	sort: (a: T, b: T) => number;
	renderItem: (record: T) => ReactNode;
}

/**
 * List is a shared component implementing the TailwindUI List spec.
 *
 * @see https://tailwindui.com/components/application-ui/lists/stacked-lists
 *
 * @template T
 * @param {ListProps<T>} props - The full prop spec for the List component
 * @param {T[]} props.dataSource - The array of data to be displayed within the list
 * @param {(a: T, b: T) => number} props.sort - The sorting function used to determine
 * list display order
 * @param {(record: T) => ReactNode} props.renderItem - The function used to render an
 * individual item from props.dataSource within the list
 *
 * @returns {Element} List
 *
 * @example
 * <List<string>
 *    dataSource={["a", "b", "c"]}
 *    sort={(a, b) => a.localCompare(b)}
 *    renderItem={(record) => <li>{record}</li>}
 * />
 */
export function List<T>(props: PropsWithChildren<ListProps<T>>): JSX.Element {
	return (
		<ul className="-my-5 divide-y divide-gray-200">
			{props?.dataSource
				?.slice()
				?.sort(props.sort)
				?.map(record => props.renderItem(record))}
		</ul>
	)
}