import React, {FC, ReactNode} from 'react';
import classNames from "classnames";

interface TooltipProps {
    tooltipContent: ReactNode;
    disabled?: boolean;
    position?: "top" | "bottom" | "left" | "right"
}

export const Tooltip: FC<TooltipProps> = (props) => {
    return (
        <div className={classNames(
            {
                "tooltip": !props.disabled
            }
        )}>
            <span className={classNames(
                "tooltip-text",
                props.position,
                // "arrow-top"
            )}>{props.tooltipContent}</span>
            {props.children}
        </div>
    );
};

Tooltip.defaultProps = {
    position: "top"
}