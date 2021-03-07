import React, {FC} from "react";
import classNames from "classnames";

export type StatisticOrientation =
	| "left"
	| "center"
	| "right";

export interface StatisticProps {
	title: string;
	value: number;
	loading?: boolean;
	orientation?: StatisticOrientation;
}

/**
 * @description Statistic is a shared component implementing the TailwindUI Statistic spec
 * @see https://tailwindui.com/components/application-ui/data-display/stats
 *
 * @param {StatisticProps} props - The full prop spec for the Statistic component
 * @param {string} props.title - The title/name of the stat being displayed @TODO: Should support ReactNode
 * @param {number} props.value - The value of the stat being displayed @TODO: Should support number/string/ReactNode
 * @param {boolean} props.loading - The current loading status of the statistic (used to render spinner)
 * @param {StatisticOrientation} props.orientation - The desired text alignment of the statistic (default = left)
 *
 * @returns {JSX.Element} Statistic - The formatted Statistic JSX.Element
 */
export const Statistic: FC<StatisticProps> = props => {
	return (
		<div>
			<div className="flex justify-between items-center px-2 py-3 sm:p-4">
				<dt className={classNames("text-base font-normal text-gray-900", {
					"text-center": props.orientation === "center",
					"text-right": props.orientation === "right"
				})}>
					{props.title}
				</dt>
				<dd className={classNames("mt-1 flex items-baseline inline-block", {
					"justify-between": !props.orientation,
					"justify-center": props.orientation === "center",
					"justify-end": props.orientation === "right"
				})}>
					<div className={classNames("flex items-baseline text-lg font-semibold text-green-500", {
						"text-center": props.orientation === "center",
						"text-right": props.orientation === "right"
					})}>
						{props.value}
					</div>
				</dd>
			</div>
		</div>
	)
}