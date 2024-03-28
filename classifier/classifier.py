import tensorflow as tf
import numpy as np

# Sample Data
data = [
    [200, 20, 0.1, 0.05, 0.0],  # normal traffic
    [404, 30, 0.2, 0.05, 0.0],  # normal traffic
    [500, 40, 0.5, 0.1, 0.3],   # malicious traffic
    [200, 25, 0.1, 0.05, 0.2],  # advertisement traffic
    [200, 22, 0.1, 0.05, 0.0],  # normal traffic
    [500, 45, 0.6, 0.2, 0.4]    # malicious traffic
]

# Corresponding Labels
labels = [0, 0, 1, 2, 0, 1]  # 0: normal, 1: malicious, 2: advertisement

data = np.array(data)
labels = np.array(labels)

model = tf.keras.models.Sequential([
    tf.keras.layers.Dense(128, activation='relu',
                          input_shape=(data.shape[1],)),
    tf.keras.layers.Dropout(0.2),
    tf.keras.layers.Dense(64, activation='relu'),
    tf.keras.layers.Dropout(0.2),
    # Assume 10 classes for classification
    tf.keras.layers.Dense(10, activation='softmax')
])

model.compile(optimizer='adam',
              loss='sparse_categorical_crossentropy',
              metrics=['accuracy'])

model.fit(data, labels, epochs=5)

model.save('traffic_classifier')


def classify(data):
    model = tf.keras.models.load_model('my_model')
    # processed_data = data_preprocessing(data)
    predictions = model.predict(data)
    return np.argmax(predictions, axis=1)  # Returns the class labels


# Assume new_data is the new HTTP response data you receive in real-time
# new_data = ...
classifications = classify(data)

print(classifications)


# Assume data_preprocessing is a function that preprocesses your raw data
# Assume label_data is a function that labels your data
# raw_data would be your collected HTTP response data

# raw_data = ...
# data = data_preprocessing(raw_data)
# labels = label_data(raw_data)
