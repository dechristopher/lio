import React, {CSSProperties, FC, ReactNode} from "react";
import classNames from "classnames";
import {BackGroundColors, bgColors} from "@utils/styles/colors";

export interface ButtonProps {
    children?: ReactNode | ReactNode[];
    removeLeftMargin?: boolean;
    roundLeftSide?: boolean;
    roundRightSide?: boolean;
    roundSides?: boolean;
    onClick?: () => void;
    style?: CSSProperties;
    bgColor?: BackGroundColors;
    selectedColor?: BackGroundColors;
    selected?: boolean;
    className?: string;
}

/**
 * Button component.
 *
 * @param {ButtonProps} props - button props
 * @param {ReactNode | ReactNode[]} props.children - the button's children
 * @param {boolean} props.removeLeftMargin - whether to apply -1 margin to the left side or not
 * @param {boolean} props.roundLeftSide - whether to round the left side or not
 * @param {boolean} props.roundRightSide - whether to round the right side or not
 * @param {() => void} props.onClick - button click handler
 *
 * @returns {Element} - button component
 *
 * @example Button
 * <Button
 *   removeLeftMargin: false,
 *   roundLeftSide: true,
 *   roundRightSide: true,
 *   onClick={() => console.log("You clicked me!")}
 * >
 *  I'm a button!
 * </Button>
 */
export const Button: FC<ButtonProps> = (props) => {
    return (
        <button
            type="button"
            onClick={props.onClick}
            style={props.style}
            className={classNames(
                "relative",
                "inline-flex",
                "items-center",
                "px-4",
                "py-2",
                "border",
                "border-gray-300",
                "text-sm",
                "font-medium",
                "text-gray-700",
                "focus:z-10",
                "focus:outline-none",
                "focus:ring-1",
                "focus:ring-indigo-500",
                "focus:border-indigo-500",
                props.bgColor,
                props.className,
                {
                    "-ml-px": props.removeLeftMargin
                },
                {
                    "rounded-l-md": props.roundLeftSide || props.roundSides
                },
                {
                    "rounded-r-md": props.roundRightSide|| props.roundSides
                },
                {
                    [props.selectedColor as string]: props.selected
                },
                {
                    "hover:bg-gray-50": !props.selected
                }
            )}>
            {props.children}
        </button>
    )
}

Button.defaultProps = {
    bgColor: "bg-white",
    className: "",
    selectedColor: bgColors.gray["200"]
}