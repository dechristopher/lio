import React, {PropsWithChildren, ReactNode} from "react";

export interface ListProps<T> {
	dataSource: T[];
	sort: (a: T, b: T) => number;
	renderItem: (record: T) => ReactNode;
}

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