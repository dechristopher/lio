import React, {PropsWithChildren, ReactNode} from "react";
import classNames from "classnames";

export declare type DataIndex = string | number | (string | number)[];

export interface ColumnProps<T> {
	title: ReactNode;
	dataIndex?: DataIndex;
	render: (record: T) => ReactNode;
}

export interface TableProps<T> {
	columns: ColumnProps<T>[];
	dataSource: T[];
}

/**
 * Table is a shared component implementing the TailwindUI Table spec.
 *
 * @see https://tailwindui.com/components/application-ui/lists/tables
 *
 * @template T
 * @param {TableProps<T>} props - The full prop spec for the Table component
 * @param {ColumnProps<T>[]} props.columns - The array list of table columns
 * @param {T[]} props.dataSource - The array of data supplied to the table
 *
 * @returns {Element} Table
 *
 * @example
 * <Table<Game>
 *		dataSource={gameData}
 *		columns={[
 *			{ title: "Player", render: (record) => record.playerName },
 *			{ title: "Rating", render: (record) => record.elo },
 *			{ title: "Mode", render: (record) => record.mode },
 *			// eslint-disable-next-line react/display-name
 *			{ title: "", render: () => <button name="join-game" className="bg-yellow-400 text-black rounded-full px-4 py-2">Join Game</button> },
 *		]}
 *	/>
 */
export function Table<T>(props: PropsWithChildren<TableProps<T>>): JSX.Element {
	return (
		<div className="flex flex-col">
			<div className="-my-2 overflow-x-auto sm:-mx-6 lg:-mx-8">
				<div className="py-2 align-middle inline-block min-w-full sm:px-6 lg:px-8">
					<div className="shadow overflow-hidden border-b border-gray-200 sm:rounded-lg">
						<table className="min-w-full divide-y divide-gray-200">
							<thead className="bg-gray-50">
								<tr>
									{props.columns.map((column, i) => (
										<th
											scope="col"
											key={`th-${i}`}
									    className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider"
										>
											{column.title}
										</th>
									))}
								</tr>
							</thead>
							<tbody>
							{props.dataSource?.map((record, i) => (
								<tr
									key={`tr-${i}`}
									className={classNames({
										"bg-white": i % 2 !== 0,
										"bg-gray-50": i % 2 === 0
									})}
								>
									{
										props.columns.map((column, c) => (
											<td
												key={`tr-${i}-c-${c}`}
												className="px-6 py-4 whitespace-nowrap text-sm text-gray-500"
											>
												{column.render(record)}
											</td>
										))
									}
								</tr>
							))}
							</tbody>
						</table>
					</div>
				</div>
			</div>
		</div>
	)
}