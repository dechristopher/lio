import React, {FC} from 'react';
import classNames from "classnames";

interface SpinnerProps {
    className?: string;
}

export const Spinner: FC<SpinnerProps> = (props) => {
    return (
        <div
            className={classNames(
                "lds-dual-ring",
                props.className
            )}
        />
    );
};